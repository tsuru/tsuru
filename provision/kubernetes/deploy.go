package kubernetes

import (
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/router"
	"k8s.io/client-go/kubernetes"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
)

func doAttach(podName, namespace, containerName string, stdin io.Reader) error {
	cfg, err := getClusterRestConfig()
	if err != nil {
		return err
	}
	cli, err := rest.RESTClientFor(cfg)
	if err != nil {
		return errors.WithStack(err)
	}
	req := cli.Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("attach")
	req.Param("container", containerName)
	req.Param("stdin", "true")
	req.Param("stdout", "true")
	req.Param("stderr", "true")
	req.Param("tty", "false")
	exec, err := remotecommand.NewExecutor(cfg, "POST", req.URL())
	if err != nil {
		return errors.WithStack(err)
	}
	err = exec.Stream(remotecommand.StreamOptions{
		SupportedProtocols: remotecommandserver.SupportedStreamingProtocols,
		Stdin:              stdin,
		Stdout:             ioutil.Discard,
		Stderr:             ioutil.Discard,
		Tty:                false,
		TerminalSizeQueue:  nil,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type buildJobParams struct {
	client           kubernetes.Interface
	app              provision.App
	buildCmd         []string
	sourceImage      string
	destinationImage string
	attachInput      io.Reader
}

func createBuildJob(params buildJobParams) (string, error) {
	parallelism := int32(1)
	dockerSockPath := "/var/run/docker.sock"
	baseName := deployJobNameForApp(params.app)
	job := &batch.Job{
		ObjectMeta: v1.ObjectMeta{
			Name:      baseName,
			Namespace: tsuruNamespace,
		},
		Spec: batch.JobSpec{
			Parallelism: &parallelism,
			Completions: &parallelism,
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Name: baseName,
					Labels: map[string]string{
						"tsuru.pod":       strconv.FormatBool(true),
						"tsuru.pod.build": strconv.FormatBool(true),
						"tsuru.app.name":  params.app.GetName(),
					},
					Annotations: map[string]string{
						"tsuru.pod.buildImage": params.destinationImage,
					},
				},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: "dockersock",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: dockerSockPath,
								},
							},
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Name:      baseName,
							Image:     params.sourceImage,
							Command:   params.buildCmd,
							Stdin:     true,
							StdinOnce: true,
						},
						{
							Name:  "committer-cont",
							Image: dockerImageName,
							VolumeMounts: []v1.VolumeMount{
								{Name: "dockersock", MountPath: dockerSockPath},
							},
							Command: []string{
								"sh", "-c",
								fmt.Sprintf(`
									while id=$(docker ps -aq -f 'label=io.kubernetes.container.name=%s' -f "label=io.kubernetes.pod.name=$(hostname)") && [ -z $id ]; do
										sleep 1;
									done;
									docker wait $id && docker commit $id %s && docker push %s
								`, baseName, params.destinationImage, params.destinationImage),
							},
						},
					},
				},
			},
		},
	}
	_, err := params.client.Batch().Jobs(tsuruNamespace).Create(job)
	if err != nil {
		return "", errors.WithStack(err)
	}
	podName, err := waitForPodRunning(params.client, baseName, baseName, defaultBuildJobTimeout)
	if err != nil {
		return "", err
	}
	if params.attachInput != nil {
		return podName, doAttach(podName, tsuruNamespace, baseName, params.attachInput)
	}
	return podName, nil
}

func extraRegisterCmds(a provision.App) string {
	host, _ := config.GetString("host")
	if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	if !strings.HasSuffix(host, "/") {
		host += "/"
	}
	token := a.Envs()["TSURU_APP_TOKEN"].Value
	return fmt.Sprintf(`curl -fsSL -m15 -XPOST -d"hostname=$(hostname)" -o/dev/null -H"Content-Type:application/x-www-form-urlencoded" -H"Authorization:bearer %s" %sapps/%s/units/register`, token, host, a.GetName())
}

func createAppDeployment(client kubernetes.Interface, oldDeployment *extensions.Deployment, a provision.App, process string, image string) error {
	replicas := int32(1)
	if oldDeployment != nil && oldDeployment.Spec.Replicas != nil {
		replicas = *oldDeployment.Spec.Replicas
	}
	routerName, err := a.GetRouterName()
	if err != nil {
		return errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return errors.WithStack(err)
	}
	extra := []string{extraRegisterCmds(a)}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, image, a, extra)
	if err != nil {
		return errors.WithStack(err)
	}
	var envs []v1.EnvVar
	for _, envData := range a.Envs() {
		envs = append(envs, v1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	host, _ := config.GetString("host")
	port := dockercommon.WebProcessDefaultPort()
	envs = append(envs, []v1.EnvVar{
		{Name: "TSURU_HOST", Value: host},
		{Name: "port", Value: port},
		{Name: "PORT", Value: port},
	}...)
	depName := deploymentNameForApp(a, process)
	deployment := extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      depName,
			Namespace: tsuruNamespace,
		},
		Spec: extensions.DeploymentSpec{
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"tsuru.pod":                  strconv.FormatBool(true),
						"tsuru.pod.build":            strconv.FormatBool(false),
						"tsuru.app.name":             a.GetName(),
						"tsuru.app.process":          process,
						"tsuru.app.process.replicas": strconv.Itoa(int(replicas)),
						"tsuru.app.platform":         a.GetPlatform(),
						"tsuru.node.pool":            a.GetPool(),
						"tsuru.router.name":          routerName,
						"tsuru.router.type":          routerType,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    depName,
							Image:   image,
							Command: cmds,
							Env:     envs,
						},
					},
				},
			},
		},
	}
	if oldDeployment == nil {
		_, err = client.Extensions().Deployments(tsuruNamespace).Create(&deployment)
	} else {
		_, err = client.Extensions().Deployments(tsuruNamespace).Update(&deployment)
	}
	return errors.WithStack(err)
}

type serviceManager struct {
	client kubernetes.Interface
}

func (m *serviceManager) RemoveService(a provision.App, process string) error {
	depName := deploymentNameForApp(a, process)
	falseVar := false
	multiErrors := tsuruErrors.NewMultiError()
	err := m.client.Extensions().Deployments(tsuruNamespace).Delete(depName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	err = m.client.Core().Services(tsuruNamespace).Delete(depName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	if multiErrors.Len() > 0 {
		return multiErrors
	}
	return waitForDeploymentDelete(m.client, depName, defaultBuildJobTimeout)
}

func (m *serviceManager) DeployService(a provision.App, process string, pState servicecommon.ProcessState, image string) error {
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.Extensions().Deployments(tsuruNamespace).Get(depName)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		dep = nil
	}
	err = createAppDeployment(m.client, dep, a, process, image)
	if err != nil {
		return err
	}
	port := dockercommon.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	_, err = m.client.Core().Services(tsuruNamespace).Create(&v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      depName,
			Namespace: tsuruNamespace,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.app.name":    a.GetName(),
				"tsuru.app.process": process,
			},
			Ports: []v1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(portInt),
					TargetPort: intstr.FromInt(portInt),
				},
			},
			Type: v1.ServiceTypeNodePort,
		},
	})
	if k8sErrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
