// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"encoding/json"
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
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	remotecommandutil "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/kubernetes/scheme"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	dockerSockPath            = "/var/run/docker.sock"
	buildIntercontainerPath   = "/tmp/intercontainer"
	buildIntercontainerStatus = buildIntercontainerPath + "/status"
	buildIntercontainerDone   = buildIntercontainerPath + "/done"
)

func doAttach(client *clusterClient, stdin io.Reader, stdout io.Writer, podName, container string) error {
	cli, err := rest.RESTClientFor(client.restConfig)
	if err != nil {
		return errors.WithStack(err)
	}
	req := cli.Post().
		Resource("pods").
		Name(podName).
		Namespace(client.Namespace()).
		SubResource("attach")
	req.VersionedParams(&apiv1.PodAttachOptions{
		Container: container,
		Stdin:     stdin != nil,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewExecutor(client.restConfig, "POST", req.URL())
	if err != nil {
		return errors.WithStack(err)
	}
	err = exec.Stream(remotecommand.StreamOptions{
		SupportedProtocols: remotecommandutil.SupportedStreamingProtocols,
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
	client           *clusterClient
	app              provision.App
	sourceImage      string
	destinationImage string
	attachInput      io.Reader
	attachOutput     io.Writer
}

func createBuildPod(params buildPodParams) error {
	baseName, err := deployPodNameForApp(params.app)
	if err != nil {
		return err
	}
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
	volumes, mounts, err := createVolumesForApp(params.client, params.app)
	if err != nil {
		return err
	}
	buildImageLabel := &provision.LabelSet{}
	buildImageLabel.SetBuildImage(params.destinationImage)
	appEnvs := provision.EnvsForApp(params.app, "", true)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   params.app.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	commitContainer := "committer-cont"
	_, uid := dockercommon.UserForContainer()
	kubeConf := getKubeConfig()
	cmds := dockercommon.ArchiveDeployCmds(params.app, "file:///home/application/archive.tar.gz")
	if len(cmds) != 3 {
		return errors.Errorf("unexpected cmds list: %#v", cmds)
	}
	cmds[2] = fmt.Sprintf(`
		cat >/home/application/archive.tar.gz && %[1]s
		exit_code=$?
		echo "${exit_code}" >%[2]s
		[ "${exit_code}" != "0" ] && exit "${exit_code}"
		while [ ! -f %[3]s ]; do sleep 1; done
	`, cmds[2], buildIntercontainerStatus, buildIntercontainerDone)
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        baseName,
			Namespace:   params.client.Namespace(),
			Labels:      labels.ToLabels(),
			Annotations: buildImageLabel.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			NodeSelector: nodeSelector,
			Volumes: append([]apiv1.Volume{
				{
					Name: "dockersock",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: dockerSockPath,
						},
					},
				},
				{
					Name: "intercontainer",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			}, volumes...),
			RestartPolicy: apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:      baseName,
					Image:     params.sourceImage,
					Command:   cmds,
					Stdin:     true,
					StdinOnce: true,
					Env:       envs,
					SecurityContext: &apiv1.SecurityContext{
						RunAsUser: uid,
					},
					VolumeMounts: append([]apiv1.VolumeMount{
						{Name: "intercontainer", MountPath: buildIntercontainerPath},
					}, mounts...),
				},
				{
					Name:  commitContainer,
					Image: kubeConf.DeploySidecarImage,
					VolumeMounts: append([]apiv1.VolumeMount{
						{Name: "dockersock", MountPath: dockerSockPath},
						{Name: "intercontainer", MountPath: buildIntercontainerPath},
					}, mounts...),
					TTY: true,
					Command: []string{
						"sh", "-ec",
						fmt.Sprintf(`
							while [ ! -f %[3]s ]; do sleep 1; done
							exit_code=$(cat %[3]s)
							[ "${exit_code}" != "0" ] && exit "${exit_code}"
							id=$(docker ps -aq -f "label=io.kubernetes.container.name=%[2]s" -f "label=io.kubernetes.pod.name=$(hostname)")
							img="%[1]s"
							echo
							echo '---- Building application image ----'
							docker commit "${id}" "${img}" >/dev/null
							sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
							echo " ---> Sending image to repository (${sz})"
							docker push "${img}"
							touch %[4]s
						`, params.destinationImage, baseName, buildIntercontainerStatus, buildIntercontainerDone),
					},
				},
			},
		},
	}
	_, err = params.client.Core().Pods(params.client.Namespace()).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	err = waitForPodContainersRunning(params.client, pod.Name, kubeConf.PodRunningTimeout)
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
	return waitForPod(params.client, pod.Name, false, kubeConf.PodReadyTimeout)
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
	return fmt.Sprintf(`curl -sSL -m15 -XPOST -d"hostname=$(hostname)" -o/dev/null -H"Content-Type:application/x-www-form-urlencoded" -H"Authorization:bearer %s" %sapps/%s/units/register || true`, token, host, a.GetName())
}

func probeFromHC(hc provision.TsuruYamlHealthcheck, port int) (*apiv1.Probe, error) {
	if hc.Path == "" {
		return nil, nil
	}
	method := strings.ToUpper(hc.Method)
	if method != "" && method != "GET" {
		return nil, errors.New("healthcheck: only GET method is supported in kubernetes provisioner")
	}
	return &apiv1.Probe{
		Handler: apiv1.Handler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path: hc.Path,
				Port: intstr.FromInt(port),
			},
		},
	}, nil
}

func createAppDeployment(client *clusterClient, oldDeployment *v1beta1.Deployment, a provision.App, process, imageName string, replicas int, labels *provision.LabelSet) (*v1beta1.Deployment, *provision.LabelSet, error) {
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
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	depName := deploymentNameForApp(a, process)
	tenRevs := int32(10)
	webProcessName, err := image.GetImageWebProcessName(imageName)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	port := provision.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	var probe *apiv1.Probe
	if process == webProcessName {
		yamlData, errImg := image.GetImageTsuruYamlData(imageName)
		if errImg != nil {
			return nil, nil, errors.WithStack(errImg)
		}
		probe, err = probeFromHC(yamlData.Healthcheck, portInt)
		if err != nil {
			return nil, nil, err
		}
	}
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   a.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	_, uid := dockercommon.UserForContainer()
	resourceLimits := apiv1.ResourceList{}
	memory := a.GetMemory()
	if memory != 0 {
		resourceLimits[apiv1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
	}
	volumes, mounts, err := createVolumesForApp(client, a)
	if err != nil {
		return nil, nil, err
	}
	deployment := v1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      depName,
			Namespace: client.Namespace(),
		},
		Spec: v1beta1.DeploymentSpec{
			Strategy: v1beta1.DeploymentStrategy{
				Type: v1beta1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &v1beta1.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &realReplicas,
			RevisionHistoryLimit: &tenRevs,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels.ToSelector(),
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels.ToLabels(),
				},
				Spec: apiv1.PodSpec{
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: uid,
					},
					RestartPolicy: apiv1.RestartPolicyAlways,
					NodeSelector:  nodeSelector,
					Volumes:       volumes,
					Containers: []apiv1.Container{
						{
							Name:           depName,
							Image:          imageName,
							Command:        cmds,
							Env:            envs,
							ReadinessProbe: probe,
							Resources: apiv1.ResourceRequirements{
								Limits: resourceLimits,
							},
							VolumeMounts: mounts,
						},
					},
				},
			},
		},
	}
	var newDep *v1beta1.Deployment
	if oldDeployment == nil {
		newDep, err = client.Extensions().Deployments(client.Namespace()).Create(&deployment)
	} else {
		newDep, err = client.Extensions().Deployments(client.Namespace()).Update(&deployment)
	}
	return newDep, labels, errors.WithStack(err)
}

type serviceManager struct {
	client *clusterClient
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) RemoveService(a provision.App, process string) error {
	multiErrors := tsuruErrors.NewMultiError()
	err := cleanupDeployment(m.client, a, process)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	depName := deploymentNameForApp(a, process)
	err = m.client.Core().Services(m.client.Namespace()).Delete(depName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
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
	dep, err := m.client.Extensions().Deployments(m.client.Namespace()).Get(depName, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return labelSetFromMeta(&dep.Spec.Template.ObjectMeta), nil
}

const deadlineExeceededProgressCond = "ProgressDeadlineExceeded"

func createDeployTimeoutError(client *clusterClient, a provision.App, processName string, w io.Writer, timeout time.Duration, label string) error {
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
	return errors.Errorf("timeout waiting %s after %v waiting for units%s", label, timeout, msgErrorPart)
}

func monitorDeployment(client *clusterClient, dep *v1beta1.Deployment, a provision.App, processName string, w io.Writer) error {
	fmt.Fprintf(w, "\n---- Updating units [%s] ----\n", processName)
	kubeConf := getKubeConfig()
	timeout := time.After(kubeConf.DeploymentProgressTimeout)
	var err error
	for dep.Status.ObservedGeneration < dep.Generation {
		dep, err = client.Extensions().Deployments(client.Namespace()).Get(dep.Name, metav1.GetOptions{})
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
			if c.Type == v1beta1.DeploymentProgressing && c.Reason == deadlineExeceededProgressCond {
				return errors.Errorf("deployment %q exceeded its progress deadline", dep.Name)
			}
		}
		if oldUpdatedReplicas != dep.Status.UpdatedReplicas {
			fmt.Fprintf(w, " ---> %d of %d new units created\n", dep.Status.UpdatedReplicas, specReplicas)
		}
		if healthcheckTimeout == nil && dep.Status.UpdatedReplicas == specReplicas {
			var allInit bool
			allInit, err = allNewPodsRunning(client, a, processName, dep.Status.ObservedGeneration)
			if allInit && err == nil {
				healthcheckTimeout = time.After(maxWaitTimeDuration)
				fmt.Fprintf(w, " ---> waiting healthcheck on %d created units\n", specReplicas)
			}
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
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0), "healthcheck")
		case <-timeout:
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0), "full rollout")
		}
		dep, err = client.Extensions().Deployments(client.Namespace()).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return nil
}

func (m *serviceManager) DeployService(a provision.App, process string, labels *provision.LabelSet, replicas int, image string) error {
	err := ensureNodeContainers()
	if err != nil {
		return err
	}
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.Extensions().Deployments(m.client.Namespace()).Get(depName, metav1.GetOptions{})
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
		rollbackErr := m.client.Extensions().Deployments(m.client.Namespace()).Rollback(&v1beta1.DeploymentRollback{
			Name: depName,
		})
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		return err
	}
	port := provision.WebProcessDefaultPort()
	portInt, _ := strconv.Atoi(port)
	_, err = m.client.Core().Services(m.client.Namespace()).Create(&apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      depName,
			Namespace: m.client.Namespace(),
			Labels:    labels.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels.ToSelector(),
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(portInt),
					TargetPort: intstr.FromInt(portInt),
				},
			},
			Type: apiv1.ServiceTypeNodePort,
		},
	})
	if k8sErrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func procfileInspectPod(client *clusterClient, a provision.App, image string) (string, error) {
	deployPodName, err := deployPodNameForApp(a)
	if err != nil {
		return "", err
	}
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return "", err
	}
	cmds := []string{"sh", "-c", "(cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile || true) 2>/dev/null"}
	buf := &bytes.Buffer{}
	err = runPod(runSinglePodArgs{
		client: client,
		stdout: buf,
		labels: labels,
		cmds:   cmds,
		name:   deployPodName,
		image:  image,
		pool:   a.GetPool(),
	})
	if err != nil {
		return "", errors.Wrapf(err, "unable to inspect Procfile: %q", buf.String())
	}
	return buf.String(), nil
}

type dockerImageSpec struct {
	Config struct {
		ExposedPorts map[string]interface{}
		Entrypoint   []string
		Cmd          []string
	}
}

func imageTagAndPush(client *clusterClient, a provision.App, oldImage, newImage string) (*dockerImageSpec, error) {
	deployPodName, err := deployPodNameForApp(a)
	if err != nil {
		return nil, err
	}
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return nil, err
	}
	kubeConf := getKubeConfig()
	buf := &bytes.Buffer{}
	err = runPod(runSinglePodArgs{
		client: client,
		stdout: buf,
		labels: labels,
		cmds: []string{"sh", "-ec", fmt.Sprintf(`
			docker pull %[1]s >/dev/null
			docker inspect %[1]s
			docker tag %[1]s %[2]s
			docker push %[2]s
`, oldImage, newImage)},
		name:       deployPodName,
		image:      kubeConf.DeployInspectImage,
		dockerSock: true,
		pool:       a.GetPool(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to pull and tag image: %q", buf.String())
	}
	var imgs []dockerImageSpec
	err = json.NewDecoder(buf).Decode(&imgs)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid image inspect response: %q", buf.String())
	}
	if len(imgs) != 1 {
		return nil, errors.Errorf("unexpected image inspect response: %q", buf.String())
	}
	return &imgs[0], nil
}
