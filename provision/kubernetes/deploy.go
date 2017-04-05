// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"k8s.io/client-go/pkg/api"
	k8sErrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
)

func doAttach(cluster *Cluster, stdin io.Reader, stdout io.Writer, podName, container string) error {
	cfg, err := cluster.getRestConfig()
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
		Namespace(cluster.namespace()).
		SubResource("attach")
	req.VersionedParams(&api.PodAttachOptions{
		Container: container,
		Stdin:     stdin != nil,
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
		Stdin:              stdin,
		Stdout:             stdout,
		Stderr:             stdout,
		Tty:                false,
		TerminalSizeQueue:  nil,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type buildPodParams struct {
	client           *Cluster
	app              provision.App
	buildCmd         []string
	sourceImage      string
	destinationImage string
	attachInput      io.Reader
	attachOutput     io.Writer
}

func createBuildPod(params buildPodParams) error {
	dockerSockPath := "/var/run/docker.sock"
	baseName := deployPodNameForApp(params.app)
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: params.app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return err
	}
	buildImageLabel := &provision.LabelSet{}
	buildImageLabel.SetBuildImage(params.destinationImage)
	appEnvs := provision.EnvsForApp(params.app, "", true)
	var envs []v1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, v1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	commitContainer := "committer-cont"
	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        baseName,
			Namespace:   params.client.namespace(),
			Labels:      labels.ToLabels(),
			Annotations: buildImageLabel.ToLabels(),
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
					Env:       envs,
				},
				{
					Name:  commitContainer,
					Image: dockerImageName,
					VolumeMounts: []v1.VolumeMount{
						{Name: "dockersock", MountPath: dockerSockPath},
					},
					Command: []string{
						"sh", "-ec",
						fmt.Sprintf(`
							img="%s"
							while id=$(docker ps -aq -f "label=io.kubernetes.container.name=%s" -f "label=io.kubernetes.pod.name=$(hostname)") && [ -z "${id}" ]; do
								sleep 1;
							done;
							docker wait "${id}" >/dev/null
							echo
							echo '---- Building application image ----'
							docker commit "${id}" "${img}" >/dev/null
							sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
							echo " ---> Sending image to repository (${sz})"
							docker push "${img}" >/dev/null
						`, params.destinationImage, baseName),
					},
				},
			},
		},
	}
	_, err = params.client.Core().Pods(params.client.namespace()).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	err = waitForPod(params.client, pod.Name, true, defaultRunPodReadyTimeout)
	if err != nil {
		return err
	}
	if params.attachInput != nil {
		errCh := make(chan error)
		go func() {
			commitErr := doAttach(params.client, nil, params.attachOutput, pod.Name, commitContainer)
			errCh <- commitErr
		}()
		err = doAttach(params.client, params.attachInput, params.attachOutput, pod.Name, baseName)
		if err != nil {
			return err
		}
		err = <-errCh
		if err != nil {
			return err
		}
		fmt.Fprintln(params.attachOutput, " ---> Cleaning up")
	}
	return waitForPod(params.client, pod.Name, false, defaultRunPodReadyTimeout)
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

func createAppDeployment(client *Cluster, oldDeployment *extensions.Deployment, a provision.App, process, imageName string, replicas int, labels *provision.LabelSet) (*extensions.Deployment, *provision.LabelSet, error) {
	provision.ExtendServiceLabels(labels, provision.ServiceLabelExtendedOpts{
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	realReplicas := int32(replicas)
	extra := []string{extraRegisterCmds(a)}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, imageName, a, extra)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	appEnvs := provision.EnvsForApp(a, process, false)
	var envs []v1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, v1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	depName := deploymentNameForApp(a, process)
	tenRevs := int32(10)
	yamlData, err := image.GetImageTsuruYamlData(imageName)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	port := provision.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	probe, err := probeFromHC(yamlData.Healthcheck, portInt)
	if err != nil {
		return nil, nil, err
	}
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool: a.GetPool(),
	}).ToNodeByPoolSelector()
	deployment := extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      depName,
			Namespace: client.namespace(),
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
					Labels: labels.ToLabels(),
				},
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyAlways,
					NodeSelector:  nodeSelector,
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
	var newDep *extensions.Deployment
	if oldDeployment == nil {
		newDep, err = client.Extensions().Deployments(client.namespace()).Create(&deployment)
	} else {
		newDep, err = client.Extensions().Deployments(client.namespace()).Update(&deployment)
	}
	return newDep, labels, errors.WithStack(err)
}

type serviceManager struct {
	client *Cluster
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) RemoveService(a provision.App, process string) error {
	falseVar := false
	multiErrors := tsuruErrors.NewMultiError()
	err := cleanupDeployment(m.client, a, process)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	depName := deploymentNameForApp(a, process)
	err = m.client.Core().Services(m.client.namespace()).Delete(depName, &v1.DeleteOptions{
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

func (m *serviceManager) CurrentLabels(a provision.App, process string) (*provision.LabelSet, error) {
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.Extensions().Deployments(m.client.namespace()).Get(depName)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return labelSetFromMeta(&dep.Spec.Template.ObjectMeta), nil
}

const deadlineExeceededProgressCond = "ProgressDeadlineExceeded"

func createDeployTimeoutError(client *Cluster, a provision.App, processName string, w io.Writer, timeout time.Duration) error {
	messages, err := notReadyPodEvents(client, a, processName)
	var msgErrorPart string
	if err == nil {
		for _, m := range messages {
			fmt.Fprintf(w, " ---> Pod not ready in time: %s\n", m)
		}
		if len(messages) > 0 {
			msgErrorPart = ": " + strings.Join(messages, ", ")
		}
	}
	return errors.Errorf("timeout after %v waiting for units%s", timeout, msgErrorPart)
}

func monitorDeployment(client *Cluster, dep *extensions.Deployment, a provision.App, processName string, w io.Writer) error {
	fmt.Fprintf(w, "\n---- Updating units [%s] ----\n", processName)
	timeout := time.After(defaultDeploymentProgressTimeout)
	var err error
	for dep.Status.ObservedGeneration < dep.Generation {
		dep, err = client.Extensions().Deployments(client.namespace()).Get(dep.Name)
		if err != nil {
			return err
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeout:
			return errors.Errorf("timeout waiting for deployment generation to update")
		}
	}
	var specReplicas int32
	if dep.Spec.Replicas != nil {
		specReplicas = *dep.Spec.Replicas
	}
	oldUpdatedReplicas := int32(-1)
	oldReadyUnits := int32(-1)
	oldPendingTermination := int32(-1)
	maxWaitTime, _ := config.GetInt("docker:healthcheck:max-time")
	if maxWaitTime == 0 {
		maxWaitTime = 120
	}
	maxWaitTimeDuration := time.Duration(maxWaitTime) * time.Second
	var healthcheckTimeout <-chan time.Time
	t0 := time.Now()
	for {
		for i := range dep.Status.Conditions {
			c := dep.Status.Conditions[i]
			if c.Type == extensions.DeploymentProgressing && c.Reason == deadlineExeceededProgressCond {
				return errors.Errorf("deployment %q exceeded its progress deadline", dep.Name)
			}
		}
		if oldUpdatedReplicas != dep.Status.UpdatedReplicas {
			fmt.Fprintf(w, " ---> %d of %d new units created\n", dep.Status.UpdatedReplicas, specReplicas)
		}
		if healthcheckTimeout == nil && dep.Status.UpdatedReplicas == specReplicas {
			healthcheckTimeout = time.After(maxWaitTimeDuration)
			fmt.Fprintf(w, " ---> waiting healthcheck on %d created units\n", specReplicas)
		}
		readyUnits := dep.Status.UpdatedReplicas - dep.Status.UnavailableReplicas
		if oldReadyUnits != readyUnits && readyUnits >= 0 {
			fmt.Fprintf(w, " ---> %d of %d new units ready\n", readyUnits, specReplicas)
		}
		pendingTermination := dep.Status.Replicas - dep.Status.UpdatedReplicas
		if oldPendingTermination != pendingTermination && pendingTermination > 0 {
			fmt.Fprintf(w, " ---> %d old units pending termination\n", pendingTermination)
		}
		oldUpdatedReplicas = dep.Status.UpdatedReplicas
		oldReadyUnits = readyUnits
		oldPendingTermination = pendingTermination
		if readyUnits == specReplicas &&
			dep.Status.Replicas == specReplicas {
			break
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-healthcheckTimeout:
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0))
		case <-timeout:
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0))
		}
		dep, err = client.Extensions().Deployments(client.namespace()).Get(dep.Name)
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return nil
}

func (m *serviceManager) DeployService(a provision.App, process string, labels *provision.LabelSet, replicas int, image string) error {
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.Extensions().Deployments(m.client.namespace()).Get(depName)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		dep = nil
	}
	dep, labels, err = createAppDeployment(m.client, dep, a, process, image, replicas, labels)
	if err != nil {
		return err
	}
	if m.writer == nil {
		m.writer = ioutil.Discard
	}
	err = monitorDeployment(m.client, dep, a, process, m.writer)
	if err != nil {
		fmt.Fprintf(m.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n ---> %s <---\n", err)
		rollbackErr := m.client.Extensions().Deployments(m.client.namespace()).Rollback(&extensions.DeploymentRollback{
			Name: depName,
		})
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		return err
	}
	port := provision.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	_, err = m.client.Core().Services(m.client.namespace()).Create(&v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      depName,
			Namespace: m.client.namespace(),
			Labels:    labels.ToLabels(),
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
