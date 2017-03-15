// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
)

func doAttach(params buildJobParams, podName, containerName string) error {
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
		Namespace(tsuruNamespace).
		SubResource("attach")
	req.VersionedParams(&api.PodAttachOptions{
		Container: containerName,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, api.ParameterCodec)
	exec, err := remotecommand.NewExecutor(cfg, "POST", req.URL())
	if err != nil {
		return errors.WithStack(err)
	}
	err = exec.Stream(remotecommand.StreamOptions{
		SupportedProtocols: remotecommandserver.SupportedStreamingProtocols,
		Stdin:              params.attachInput,
		Stdout:             params.attachOutput,
		Stderr:             params.attachOutput,
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
	attachOutput     io.Writer
}

func createBuildJob(params buildJobParams) (string, error) {
	parallelism := int32(1)
	dockerSockPath := "/var/run/docker.sock"
	baseName := deployJobNameForApp(params.app)
	labels, err := podLabels(params.app, "", params.destinationImage, 0)
	if err != nil {
		return "", err
	}
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
					Name:        baseName,
					Labels:      labels.ToLabels(),
					Annotations: labels.ToAnnotations(),
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
	createdJob, err := params.client.Batch().Jobs(tsuruNamespace).Create(job)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if createdJob.Spec.Selector == nil {
		return "", errors.Errorf("empty selector for created job %q", job.Name)
	}
	podName, err := waitForJobContainerRunning(params.client, createdJob.Spec.Selector.MatchLabels, baseName, defaultBuildJobTimeout)
	if err != nil {
		return "", err
	}
	if params.attachInput != nil {
		return podName, doAttach(params, podName, baseName)
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

func probeFromHC(hc provision.TsuruYamlHealthcheck, port int) (*v1.Probe, error) {
	if hc.Path == "" {
		return nil, nil
	}
	method := strings.ToUpper(hc.Method)
	if method != "" && method != "GET" {
		return nil, errors.New("healthcheck: only GET method is supported in kubernetes provisioner")
	}
	return &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path: hc.Path,
				Port: intstr.FromInt(port),
			},
		},
	}, nil
}

func createAppDeployment(client kubernetes.Interface, oldDeployment *extensions.Deployment, a provision.App, process, imageName string, pState servicecommon.ProcessState) (*labelSet, error) {
	replicas := 0
	restartCount := 0
	if oldDeployment != nil {
		oldLabels := labelSetFromMeta(&oldDeployment.Spec.Template.ObjectMeta)
		// Use label instead of .Spec.Replicas because stopped apps will have
		// always 0 .Spec.Replicas.
		replicas = oldLabels.AppReplicas()
		restartCount = oldLabels.Restarts()
	}
	if pState.Increment != 0 {
		replicas += pState.Increment
		if replicas < 0 {
			return nil, errors.New("cannot have less than 0 units")
		}
	} else if replicas == 0 && pState.Start {
		replicas = 1
	}
	extra := []string{extraRegisterCmds(a)}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, imageName, a, extra)
	if err != nil {
		return nil, errors.WithStack(err)
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
	tenRevs := int32(10)
	labels, err := podLabels(a, process, "", replicas)
	if err != nil {
		return nil, err
	}
	realReplicas := int32(replicas)
	if pState.Stop {
		realReplicas = 0
	}
	if pState.Restart {
		restartCount++
		labels.SetRestarts(restartCount)
	}
	yamlData, err := image.GetImageTsuruYamlData(imageName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	portInt, _ := strconv.Atoi(port)
	probe, err := probeFromHC(yamlData.Healthcheck, portInt)
	if err != nil {
		return nil, err
	}
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	deployment := extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      depName,
			Namespace: tsuruNamespace,
		},
		Spec: extensions.DeploymentSpec{
			Strategy: extensions.DeploymentStrategy{
				Type: extensions.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &extensions.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &realReplicas,
			RevisionHistoryLimit: &tenRevs,
			Selector: &unversioned.LabelSelector{
				MatchLabels: labels.ToSelector(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      labels.ToLabels(),
					Annotations: labels.ToAnnotations(),
				},
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{
						labelNodePoolName: a.GetPool(),
					},
					Containers: []v1.Container{
						{
							Name:           depName,
							Image:          imageName,
							Command:        cmds,
							Env:            envs,
							ReadinessProbe: probe,
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
	return labels, errors.WithStack(err)
}

type serviceManager struct {
	client kubernetes.Interface
}

func (m *serviceManager) RemoveService(a provision.App, process string) error {
	falseVar := false
	multiErrors := tsuruErrors.NewMultiError()
	err := cleanupDeployment(m.client, a, process)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	depName := deploymentNameForApp(a, process)
	err = m.client.Core().Services(tsuruNamespace).Delete(depName, &v1.DeleteOptions{
		OrphanDependents: &falseVar,
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	if multiErrors.Len() > 0 {
		return multiErrors
	}
	return nil
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
	labels, err := createAppDeployment(m.client, dep, a, process, image, pState)
	if err != nil {
		return err
	}
	port := dockercommon.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	_, err = m.client.Core().Services(tsuruNamespace).Create(&v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        depName,
			Namespace:   tsuruNamespace,
			Labels:      labels.ToLabels(),
			Annotations: labels.ToAnnotations(),
		},
		Spec: v1.ServiceSpec{
			Selector: labels.ToSelector(),
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
