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
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/api/apps/v1beta2"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	dockerSockPath            = "/var/run/docker.sock"
	buildIntercontainerPath   = "/tmp/intercontainer"
	buildIntercontainerStatus = buildIntercontainerPath + "/status"
	buildIntercontainerDone   = buildIntercontainerPath + "/done"
)

func keepAliveSpdyExecutor(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}
	upgradeRoundTripper := spdy.NewSpdyRoundTripper(tlsConfig, true)
	upgradeRoundTripper.Dialer = &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 10 * time.Second,
	}
	wrapper, err := rest.HTTPWrappersForConfig(config, upgradeRoundTripper)
	if err != nil {
		return nil, err
	}
	return remotecommand.NewSPDYExecutorForTransports(wrapper, upgradeRoundTripper, method, url)
}

func doAttach(client *ClusterClient, stdin io.Reader, stdout, stderr io.Writer, podName, container string, tty bool) error {
	cli, err := rest.RESTClientFor(client.restConfig)
	if err != nil {
		return errors.WithStack(err)
	}
	req := cli.Post().
		Resource("pods").
		Name(podName).
		Namespace(client.Namespace()).
		SubResource("attach")
	// Attaching stderr is only allowed if tty == false, otherwise the attach
	// call will fail.
	req.VersionedParams(&apiv1.PodAttachOptions{
		Container: container,
		Stdin:     stdin != nil,
		Stdout:    true,
		Stderr:    !tty,
		TTY:       tty,
	}, scheme.ParameterCodec)
	exec, err := keepAliveSpdyExecutor(client.restConfig, "POST", req.URL())
	if err != nil {
		return errors.WithStack(err)
	}
	var sizeQueue remotecommand.TerminalSizeQueue
	if tty {
		sizeQueue = &fixedSizeQueue{
			sz: &remotecommand.TerminalSize{
				Width:  1000,
				Height: 1000,
			},
		}
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               tty,
		TerminalSizeQueue: sizeQueue,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type createPodParams struct {
	client           *ClusterClient
	app              provision.App
	podName          string
	cmds             []string
	sidecarCmds      []string
	sourceImage      string
	destinationImage string
	inputFile        string
	attachInput      io.Reader
	attachOutput     io.Writer
}

func createBuildPod(params createPodParams) error {
	cmds := dockercommon.ArchiveBuildCmds(params.app, "file:///home/application/archive.tar.gz")
	if params.podName == "" {
		var err error
		if params.podName, err = buildPodNameForApp(params.app, ""); err != nil {
			return err
		}
	}
	params.cmds = cmds
	return createPod(params)
}

func createDeployPod(params createPodParams) error {
	cmds := dockercommon.DeployCmds(params.app)
	if params.podName == "" {
		var err error
		if params.podName, err = deployPodNameForApp(params.app); err != nil {
			return err
		}
	}
	params.cmds = cmds
	repository, tag := image.SplitImageName(params.destinationImage)
	if tag != "latest" {
		params.sidecarCmds = []string{
			fmt.Sprintf(`
				docker tag %[1]s %[2]s:latest
				docker push %[2]s:latest
			`, params.destinationImage, repository),
		}
	}
	return createPod(params)
}

func createPod(params createPodParams) error {
	if len(params.cmds) != 3 {
		return errors.Errorf("unexpected cmds list: %#v", params.cmds)
	}
	params.cmds[2] = fmt.Sprintf(`
		cat >%[4]s && %[1]s
		exit_code=$?
		echo "${exit_code}" >%[2]s
		[ "${exit_code}" != "0" ] && exit "${exit_code}"
		while [ ! -f %[3]s ]; do sleep 1; done
	`, params.cmds[2], buildIntercontainerStatus, buildIntercontainerDone, params.inputFile)
	err := ensureServiceAccountForApp(params.client, params.app)
	if err != nil {
		return err
	}
	baseName := params.podName
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
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	volumes, mounts, err := createVolumesForApp(params.client, params.app)
	if err != nil {
		return err
	}
	annotations.SetBuildImage(params.destinationImage)
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
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        baseName,
			Namespace:   params.client.Namespace(),
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: serviceAccountNameForApp(params.app),
			NodeSelector:       nodeSelector,
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
					Command:   params.cmds,
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
						strings.Join(
							append([]string{fmt.Sprintf(`
							end() { touch %[4]s; }
							trap end EXIT
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
							%[5]s
							docker push %[1]s
						`, params.destinationImage, baseName, buildIntercontainerStatus, buildIntercontainerDone, registryAuth(params.destinationImage))}, params.sidecarCmds...),
							"\n"),
					},
				},
			},
		},
	}
	_, err = params.client.CoreV1().Pods(params.client.Namespace()).Create(pod)
	if err != nil {
		return errors.WithStack(err)
	}
	watch, err := filteredPodEvents(params.client, "", params.podName)
	if err != nil {
		return err
	}
	watchDone := make(chan struct{})
	defer func() {
		watch.Stop()
		<-watchDone
	}()
	watchCh := watch.ResultChan()
	go func() {
		defer close(watchDone)
		for {
			msg, isOpen := <-watchCh
			if !isOpen {
				return
			}
			fmt.Fprintf(params.attachOutput, " ---> %s\n", formatEvtMessage(msg, true))
		}
	}()
	err = waitForPodContainersRunning(params.client, pod.Name, kubeConf.PodRunningTimeout)
	if err != nil {
		return err
	}
	if params.attachInput != nil {
		errCh := make(chan error)
		go func() {
			commitErr := doAttach(params.client, nil, params.attachOutput, params.attachOutput, pod.Name, commitContainer, true)
			errCh <- commitErr
		}()
		err = doAttach(params.client, params.attachInput, params.attachOutput, params.attachOutput, pod.Name, baseName, false)
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

func registryAuth(img string) string {
	imgDomain := strings.Split(img, "/")[0]
	r, _ := config.Get("docker:registry")
	if imgDomain != r {
		return ""
	}
	username, _ := config.GetString("docker:registry-auth:username")
	password, _ := config.GetString("docker:registry-auth:password")
	if len(username) == 0 && len(password) == 0 {
		return ""
	}

	return fmt.Sprintf(`docker login -u "%s" -p "%s" "%s"`, username, password, imgDomain)
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
	scheme := hc.Scheme
	if scheme == "" {
		scheme = provision.DefaultHealthcheckScheme
	}
	scheme = strings.ToUpper(scheme)
	method := strings.ToUpper(hc.Method)
	if method != "" && method != "GET" {
		return nil, errors.New("healthcheck: only GET method is supported in kubernetes provisioner")
	}
	return &apiv1.Probe{
		FailureThreshold: int32(hc.AllowedFailures),
		Handler: apiv1.Handler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:   hc.Path,
				Port:   intstr.FromInt(port),
				Scheme: apiv1.URIScheme(scheme),
			},
		},
	}, nil
}

func ensureServiceAccount(client *ClusterClient, name string, labels *provision.LabelSet) error {
	svcAccount := apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels.ToLabels(),
		},
	}
	_, err := client.CoreV1().ServiceAccounts(client.Namespace()).Create(&svcAccount)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return errors.WithStack(err)
	}
	return nil
}

func ensureServiceAccountForApp(client *ClusterClient, a provision.App) error {
	labels := provision.ServiceAccountLabels(provision.ServiceAccountLabelsOpts{
		App:         a,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	return ensureServiceAccount(client, serviceAccountNameForApp(a), labels)
}

func createAppDeployment(client *ClusterClient, oldDeployment *v1beta2.Deployment, a provision.App, process, imageName string, replicas int, labels *provision.LabelSet) (*v1beta2.Deployment, *provision.LabelSet, *provision.LabelSet, error) {
	provision.ExtendServiceLabels(labels, provision.ServiceLabelExtendedOpts{
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	realReplicas := int32(replicas)
	extra := []string{extraRegisterCmds(a)}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, imageName, a, extra)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
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
		return nil, nil, nil, errors.WithStack(err)
	}
	portInt := getTargetPortForImage(imageName)
	var probe *apiv1.Probe
	if process == webProcessName {
		yamlData, errImg := image.GetImageTsuruYamlData(imageName)
		if errImg != nil {
			return nil, nil, nil, errors.WithStack(errImg)
		}
		probe, err = probeFromHC(yamlData.Healthcheck, portInt)
		if err != nil {
			return nil, nil, nil, err
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
	overcommit, err := client.OvercommitFactor(a.GetPool())
	if err != nil {
		return nil, nil, nil, errors.WithMessage(err, "misconfigured cluster overcommit factor")
	}
	resourceRequests := apiv1.ResourceList{}
	memory := a.GetMemory()
	if memory != 0 {
		resourceLimits[apiv1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
		resourceRequests[apiv1.ResourceMemory] = *resource.NewQuantity(memory/overcommit, resource.BinarySI)
	}
	volumes, mounts, err := createVolumesForApp(client, a)
	if err != nil {
		return nil, nil, nil, err
	}
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	deployment := v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   client.Namespace(),
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: v1beta2.DeploymentSpec{
			Strategy: v1beta2.DeploymentStrategy{
				Type: v1beta2.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &v1beta2.RollingUpdateDeployment{
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
					Labels:      labels.ToLabels(),
					Annotations: annotations.ToLabels(),
				},
				Spec: apiv1.PodSpec{
					ServiceAccountName: serviceAccountNameForApp(a),
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: uid,
					},
					RestartPolicy: apiv1.RestartPolicyAlways,
					NodeSelector:  nodeSelector,
					Volumes:       volumes,
					Subdomain:     headlessServiceNameForApp(a, process),
					Containers: []apiv1.Container{
						{
							Name:           depName,
							Image:          imageName,
							Command:        cmds,
							Env:            envs,
							ReadinessProbe: probe,
							LivenessProbe:  probe,
							Resources: apiv1.ResourceRequirements{
								Limits:   resourceLimits,
								Requests: resourceRequests,
							},
							VolumeMounts: mounts,
							Ports: []apiv1.ContainerPort{
								{ContainerPort: int32(portInt)},
							},
						},
					},
				},
			},
		},
	}
	var newDep *v1beta2.Deployment
	if oldDeployment == nil {
		newDep, err = client.AppsV1beta2().Deployments(client.Namespace()).Create(&deployment)
	} else {
		newDep, err = client.AppsV1beta2().Deployments(client.Namespace()).Update(&deployment)
	}
	return newDep, labels, annotations, errors.WithStack(err)
}

type serviceManager struct {
	client *ClusterClient
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
	err = m.client.CoreV1().Services(m.client.Namespace()).Delete(depName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	headlessSvcName := headlessServiceNameForApp(a, process)
	err = m.client.CoreV1().Services(m.client.Namespace()).Delete(headlessSvcName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	return multiErrors.ToError()
}

func (m *serviceManager) CurrentLabels(a provision.App, process string) (*provision.LabelSet, error) {
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.AppsV1beta2().Deployments(m.client.Namespace()).Get(depName, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return labelSetFromMeta(&dep.Spec.Template.ObjectMeta), nil
}

const deadlineExeceededProgressCond = "ProgressDeadlineExceeded"

func createDeployTimeoutError(client *ClusterClient, a provision.App, processName string, w io.Writer, timeout time.Duration, label string) error {
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

func filteredPodEvents(client *ClusterClient, evtResourceVersion, podName string) (watch.Interface, error) {
	var err error
	client, err = NewClusterClient(client.Cluster)
	if err != nil {
		return nil, err
	}
	err = client.SetTimeout(time.Hour)
	if err != nil {
		return nil, err
	}
	selector := map[string]string{
		"involvedObject.kind": "Pod",
	}
	if podName != "" {
		selector["involvedObject.name"] = podName
	}
	evtWatch, err := client.CoreV1().Events(client.Namespace()).Watch(metav1.ListOptions{
		FieldSelector:   labels.SelectorFromSet(labels.Set(selector)).String(),
		Watch:           true,
		ResourceVersion: evtResourceVersion,
	})
	if err != nil {
		return nil, err
	}
	return evtWatch, nil
}

func isDeploymentEvent(msg watch.Event, dep *v1beta2.Deployment) bool {
	evt, ok := msg.Object.(*apiv1.Event)
	return ok && strings.HasPrefix(evt.Name, dep.Name)
}

func formatEvtMessage(msg watch.Event, showSub bool) string {
	evt, ok := msg.Object.(*apiv1.Event)
	if !ok {
		return ""
	}
	var subStr string
	if showSub && evt.InvolvedObject.FieldPath != "" {
		subStr = fmt.Sprintf(" - %s", evt.InvolvedObject.FieldPath)
	}
	component := []string{evt.Source.Component}
	if evt.Source.Host != "" {
		component = append(component, evt.Source.Host)
	}
	return fmt.Sprintf("%s%s - %s [%s]",
		evt.InvolvedObject.Name,
		subStr,
		evt.Message,
		strings.Join(component, ", "),
	)
}

func monitorDeployment(client *ClusterClient, dep *v1beta2.Deployment, a provision.App, processName string, w io.Writer, evtResourceVersion string) error {
	watch, err := filteredPodEvents(client, evtResourceVersion, "")
	if err != nil {
		return err
	}
	watchCh := watch.ResultChan()
	defer func() {
		watch.Stop()
		if watchCh != nil {
			// Drain watch channel to avoid goroutine leaks.
			<-watchCh
		}
	}()
	fmt.Fprintf(w, "\n---- Updating units [%s] ----\n", processName)
	kubeConf := getKubeConfig()
	timeout := time.After(kubeConf.DeploymentProgressTimeout)
	for dep.Status.ObservedGeneration < dep.Generation {
		dep, err = client.AppsV1beta2().Deployments(client.Namespace()).Get(dep.Name, metav1.GetOptions{})
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
			if c.Type == v1beta2.DeploymentProgressing && c.Reason == deadlineExeceededProgressCond {
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
		case msg, isOpen := <-watchCh:
			if !isOpen {
				watchCh = nil
				break
			}
			if isDeploymentEvent(msg, dep) {
				fmt.Fprintf(w, "  ---> %s\n", formatEvtMessage(msg, false))
			}
		case <-healthcheckTimeout:
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0), "healthcheck")
		case <-timeout:
			return createDeployTimeoutError(client, a, processName, w, time.Since(t0), "full rollout")
		}
		dep, err = client.AppsV1beta2().Deployments(client.Namespace()).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return nil
}

func (m *serviceManager) DeployService(a provision.App, process string, labels *provision.LabelSet, replicas int, img string) error {
	err := ensureNodeContainers()
	if err != nil {
		return err
	}
	err = ensureServiceAccountForApp(m.client, a)
	if err != nil {
		return err
	}
	depName := deploymentNameForApp(a, process)
	dep, err := m.client.AppsV1beta2().Deployments(m.client.Namespace()).Get(depName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		dep = nil
	}
	events, err := m.client.CoreV1().Events(m.client.Namespace()).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	dep, labels, annotations, err := createAppDeployment(m.client, dep, a, process, img, replicas, labels)
	if err != nil {
		return err
	}
	if m.writer == nil {
		m.writer = ioutil.Discard
	}
	err = monitorDeployment(m.client, dep, a, process, m.writer, events.ResourceVersion)
	if err != nil {
		fmt.Fprintf(m.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n ---> %s <---\n", err)
		rollbackErr := m.client.ExtensionsV1beta1().Deployments(m.client.Namespace()).Rollback(&extensions.DeploymentRollback{
			Name: depName,
		})
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		return provision.ErrUnitStartup{Err: err}
	}
	targetPort := getTargetPortForImage(img)
	port, _ := strconv.Atoi(provision.WebProcessDefaultPort())
	_, err = m.client.CoreV1().Services(m.client.Namespace()).Create(&apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   m.client.Namespace(),
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels.ToSelector(),
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(port),
					TargetPort: intstr.FromInt(targetPort),
				},
			},
			Type: apiv1.ServiceTypeNodePort,
		},
	})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}
	labels.SetIsHeadlessService()
	_, err = m.client.CoreV1().Services(m.client.Namespace()).Create(&apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        headlessServiceNameForApp(a, process),
			Namespace:   m.client.Namespace(),
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels.ToSelector(),
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(port),
					TargetPort: intstr.FromInt(targetPort),
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func getTargetPortForImage(imgName string) int {
	port := provision.WebProcessDefaultPort()
	imageData, _ := image.GetImageMetaData(imgName)
	if imageData.ExposedPort != "" {
		parts := strings.SplitN(imageData.ExposedPort, "/", 2)
		if len(parts) == 2 {
			port = parts[0]
		}
	}
	portInt, _ := strconv.Atoi(port)
	return portInt
}

func procfileInspectPod(client *ClusterClient, a provision.App, image string) (string, error) {
	deployPodName, err := buildPodNameForApp(a, "procfile-inspect")
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
	cmd := "(cat /home/application/current/Procfile || cat /app/user/Procfile || cat /Procfile || true) 2>/dev/null"
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = runPod(runSinglePodArgs{
		client: client,
		stdout: stdout,
		stderr: stderr,
		labels: labels,
		cmd:    cmd,
		name:   deployPodName,
		image:  image,
		app:    a,
	})
	if err != nil {
		return "", errors.Wrapf(err, "unable to inspect Procfile: stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}
	return stdout.String(), nil
}

func imageTagAndPush(client *ClusterClient, a provision.App, oldImage, newImage string) (*docker.Image, error) {
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
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = runPod(runSinglePodArgs{
		client: client,
		stdout: stdout,
		stderr: stderr,
		labels: labels,
		cmd: fmt.Sprintf(`
			%[1]s
			docker pull %[2]s >/dev/null
			docker inspect %[2]s
			docker tag %[2]s %[4]s
			%[3]s
			docker push %[4]s
`, registryAuth(oldImage), oldImage, registryAuth(newImage), newImage),
		name:       deployPodName,
		image:      kubeConf.DeployInspectImage,
		dockerSock: true,
		app:        a,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to pull and tag image: stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}
	var imgs []docker.Image
	bufData := stdout.String()
	err = json.NewDecoder(stdout).Decode(&imgs)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid image inspect response: %q", bufData)
	}
	if len(imgs) != 1 {
		return nil, errors.Errorf("unexpected image inspect response: %q", bufData)
	}
	return &imgs[0], nil
}

func loadTsuruYamlPod(client *ClusterClient, a provision.App, image string) (*provision.TsuruYamlData, error) {
	const path = "/home/application/current"
	deployPodName, err := buildPodNameForApp(a, "yamldata")
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
	cmdCat := fmt.Sprintf("(cat %[1]s/tsuru.yml || cat %[1]s/tsuru.yaml || cat %[1]s/app.yml || cat %[1]s/app.yaml || true) 2>/dev/null", path)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = runPod(runSinglePodArgs{
		client: client,
		stdout: stdout,
		stderr: stderr,
		labels: labels,
		cmd:    cmdCat,
		name:   deployPodName,
		image:  image,
		app:    a,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to inspect tsuru.yml. stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}
	var tsuruYamlData provision.TsuruYamlData
	bufData := stdout.String()
	err = yaml.Unmarshal(stdout.Bytes(), &tsuruYamlData)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid load tsuru yaml response: %q", bufData)
	}
	return &tsuruYamlData, nil
}
