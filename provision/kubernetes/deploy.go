// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/servicecommon"
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
	dockerSockPath          = "/var/run/docker.sock"
	buildIntercontainerPath = "/tmp/intercontainer"
	buildIntercontainerDone = buildIntercontainerPath + "/done"
)

var (
	defaultGracePeriodSeconds int64 = 120
)

type InspectData struct {
	Image     docker.Image
	TsuruYaml provision.TsuruYamlData
	Procfile  string
}

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

func doAttach(ctx context.Context, client *ClusterClient, stdin io.Reader, stdout, stderr io.Writer, podName, container string, tty bool, size *remotecommand.TerminalSize, namespace string) error {
	errCh := make(chan error, 2)
	go func() {
		errCh <- doUnsafeAttach(client, stdin, stdout, stderr, podName, container, tty, size, namespace)
	}()
	finishedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		err := waitForContainerFinished(finishedCtx, client, podName, container, namespace)
		if err == nil {
			// The attach call and the waitForContainer may happen at the same
			// time this timeout ensures we only raise an error if attach is
			// really blocked after the container finished.
			conf := getKubeConfig()
			time.Sleep(conf.AttachTimeoutAfterContainerFinished)
			errCh <- errors.New("container finished while attach is running")
		} else {
			log.Errorf("error while waiting for container to finish during attach, attach not canceled: %v", err)
		}
	}()
	// WARNING(cezarsa): If a context cancellation or a container finished
	// situation is triggered there's no reliable way to close the pending
	// doUnsafeAttach call. We may only hope it will be gone eventually (as it
	// should if the remote host isn't accessible anymore due to tcp keepalive
	// probes failing). If it takes too long to finish or if too many happen in
	// a short period of time we may leak sockets here.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func doUnsafeAttach(client *ClusterClient, stdin io.Reader, stdout, stderr io.Writer, podName, container string, tty bool, size *remotecommand.TerminalSize, namespace string) error {
	cli, err := rest.RESTClientFor(client.restConfig)
	if err != nil {
		return errors.WithStack(err)
	}
	req := cli.Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
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
		if size == nil {
			size = &remotecommand.TerminalSize{
				Width:  1000,
				Height: 1000,
			}
		}
		sizeQueue = &fixedSizeQueue{
			sz: size,
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
	client            *ClusterClient
	app               provision.App
	podName           string
	cmds              []string
	sourceImage       string
	destinationImages []string
	inputFile         string
	attachInput       io.Reader
	attachOutput      io.Writer
	pod               *apiv1.Pod
	mainContainer     string
}

func createBuildPod(ctx context.Context, params createPodParams) error {
	cmds := dockercommon.ArchiveBuildCmds(params.app, "file:///home/application/archive.tar.gz")
	if params.podName == "" {
		var err error
		if params.podName, err = buildPodNameForApp(params.app); err != nil {
			return err
		}
	}
	params.cmds = cmds
	return createPod(ctx, params)
}

func createDeployPod(ctx context.Context, params createPodParams) error {
	if len(params.destinationImages) == 0 {
		return fmt.Errorf("no destination images provided")
	}
	cmds := dockercommon.DeployCmds(params.app)
	if params.podName == "" {
		var err error
		if params.podName, err = deployPodNameForApp(params.app); err != nil {
			return err
		}
	}
	params.cmds = cmds
	repository, tag := image.SplitImageName(params.destinationImages[0])
	if tag != "latest" {
		params.destinationImages = append(params.destinationImages, fmt.Sprintf("%s:latest", repository))
	}
	return createPod(ctx, params)
}

func createImageBuildPod(ctx context.Context, params createPodParams) error {
	params.mainContainer = "build-cont"
	kubeConf := getKubeConfig()
	pod, err := newDeployAgentImageBuildPod(params.client, params.sourceImage, params.podName, deployAgentConfig{
		name:              params.mainContainer,
		image:             kubeConf.DeploySidecarImage,
		cmd:               fmt.Sprintf("mkdir -p $(dirname %[1]s) && cat >%[1]s && tsuru_unit_agent", params.inputFile),
		destinationImages: params.destinationImages,
		inputFile:         params.inputFile,
		dockerfileBuild:   true,
	})
	if err != nil {
		return err
	}
	params.pod = &pod
	return createPod(ctx, params)
}

func getImagePullSecrets(client *ClusterClient, namespace string, images ...string) ([]apiv1.LocalObjectReference, error) {
	registry, _ := config.GetString("docker:registry")
	useSecret := false
	for _, image := range images {
		imgDomain := strings.Split(image, "/")[0]
		if imgDomain == registry {
			useSecret = true
			break
		}
	}
	if !useSecret {
		return nil, nil
	}
	err := ensureAuthSecret(client, namespace)
	if err != nil {
		return nil, err
	}
	secretName := registrySecretName(registry)
	return []apiv1.LocalObjectReference{
		{Name: secretName},
	}, nil
}

func ensureAuthSecret(client *ClusterClient, namespace string) error {
	registry, _ := config.GetString("docker:registry")
	username, _ := config.GetString("docker:registry-auth:username")
	password, _ := config.GetString("docker:registry-auth:password")
	if len(username) == 0 && len(password) == 0 {
		return nil
	}
	authEncoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	conf := map[string]map[string]dockerTypes.AuthConfig{
		"auths": {
			registry: {
				Username: username,
				Password: password,
				Auth:     authEncoded,
			},
		},
	}
	serializedConf, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrySecretName(registry),
		},
		Data: map[string][]byte{
			apiv1.DockerConfigJsonKey: serializedConf,
		},
		Type: apiv1.SecretTypeDockerConfigJson,
	}
	_, err = client.CoreV1().Secrets(namespace).Update(secret)
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = client.CoreV1().Secrets(namespace).Create(secret)
	}
	if err != nil {
		err = errors.WithStack(err)
	}
	return err
}

func createPod(ctx context.Context, params createPodParams) error {
	if params.mainContainer == "" {
		params.mainContainer = "committer-cont"
	}
	kubeConf := getKubeConfig()
	if params.pod == nil {
		if len(params.cmds) != 3 {
			return errors.Errorf("unexpected cmds list: %#v", params.cmds)
		}
		pod, err := newDeployAgentPod(params.client, params.sourceImage, params.app, params.podName, deployAgentConfig{
			name:              params.mainContainer,
			image:             kubeConf.DeploySidecarImage,
			cmd:               fmt.Sprintf("mkdir -p $(dirname %[1]s) && cat >%[1]s && %[2]s", params.inputFile, strings.Join(params.cmds[2:], " ")),
			destinationImages: params.destinationImages,
			inputFile:         params.inputFile,
		})
		if err != nil {
			return err
		}
		params.pod = &pod
	}
	ns, err := params.client.AppNamespace(params.app)
	if err != nil {
		return err
	}
	events, err := params.client.CoreV1().Events(ns).List(listOptsForPodEvent(params.podName))
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = params.client.CoreV1().Pods(ns).Create(params.pod)
	if err != nil {
		return errors.WithStack(err)
	}
	watch, err := filteredPodEvents(params.client, events.ResourceVersion, params.podName, ns)
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
	tctx, cancel := context.WithTimeout(ctx, kubeConf.PodRunningTimeout)
	err = waitForPodContainersRunning(tctx, params.client, params.pod, ns)
	cancel()
	if err != nil {
		return err
	}
	if params.attachInput != nil {
		err = doAttach(ctx, params.client, params.attachInput, params.attachOutput, params.attachOutput, params.pod.Name, params.mainContainer, false, nil, ns)
		if err != nil {
			return fmt.Errorf("error attaching to %s/%s: %v", params.pod.Name, params.mainContainer, err)
		}
		fmt.Fprintln(params.attachOutput, " ---> Cleaning up")
	}
	tctx, cancel = context.WithTimeout(ctx, kubeConf.PodReadyTimeout)
	defer cancel()
	return waitForPod(tctx, params.client, params.pod, ns, false)
}

func registryAuth(img string) (username, password, imgDomain string) {
	imgDomain = strings.Split(img, "/")[0]
	r, _ := config.GetString("docker:registry")
	if imgDomain != r {
		return "", "", ""
	}
	username, _ = config.GetString("docker:registry-auth:username")
	password, _ = config.GetString("docker:registry-auth:password")
	if len(username) == 0 && len(password) == 0 {
		return "", "", ""
	}
	return username, password, imgDomain
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

type hcResult struct {
	liveness  *apiv1.Probe
	readiness *apiv1.Probe
}

func probesFromHC(hc provision.TsuruYamlHealthcheck, port int) (hcResult, error) {
	var result hcResult
	if hc.Path == "" {
		return result, nil
	}
	if hc.Scheme == "" {
		hc.Scheme = provision.DefaultHealthcheckScheme
	}
	hc.Method = strings.ToUpper(hc.Method)
	if hc.Method == "" {
		hc.Method = http.MethodGet
	}
	if hc.IntervalSeconds == 0 {
		hc.IntervalSeconds = 10
	}
	if hc.TimeoutSeconds == 0 {
		hc.TimeoutSeconds = 60
	}
	if !hc.UseInRouter {
		url := fmt.Sprintf("%s://localhost:%d/%s", hc.Scheme, port, strings.TrimPrefix(hc.Path, "/"))
		result.readiness = &apiv1.Probe{
			FailureThreshold: int32(hc.AllowedFailures),
			PeriodSeconds:    int32(3),
			TimeoutSeconds:   int32(hc.TimeoutSeconds),
			Handler: apiv1.Handler{
				Exec: &apiv1.ExecAction{
					Command: []string{
						"sh", "-c",
						fmt.Sprintf(`if [ ! -f /tmp/onetimeprobesuccessful ]; then curl -ksSf -X%[1]s -o /dev/null %[2]s && touch /tmp/onetimeprobesuccessful; fi`,
							hc.Method, url),
					},
				},
			},
		}
		return result, nil
	}
	if hc.Method != http.MethodGet {
		return result, errors.New("healthcheck: only GET method is supported in kubernetes provisioner with use_in_router set")
	}
	if hc.AllowedFailures == 0 {
		hc.AllowedFailures = 3
	}
	hc.Scheme = strings.ToUpper(hc.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: int32(hc.AllowedFailures),
		PeriodSeconds:    int32(hc.IntervalSeconds),
		TimeoutSeconds:   int32(hc.TimeoutSeconds),
		Handler: apiv1.Handler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:   hc.Path,
				Port:   intstr.FromInt(port),
				Scheme: apiv1.URIScheme(hc.Scheme),
			},
		},
	}
	result.readiness = probe
	if hc.ForceRestart {
		result.liveness = probe
	}
	return result, nil
}

func ensureNamespaceForApp(client *ClusterClient, app provision.App) error {
	ns, err := client.AppNamespace(app)
	if err != nil {
		return err
	}
	return ensureNamespace(client, ns)
}

func ensurePoolNamespace(client *ClusterClient, pool string) error {
	return ensureNamespace(client, client.PoolNamespace(pool))
}

func ensureNamespace(client *ClusterClient, namespace string) error {
	nsLabels, err := client.namespaceLabels(namespace)
	if err != nil {
		return err
	}
	ns := apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: nsLabels,
		},
	}
	_, err = client.CoreV1().Namespaces().Create(&ns)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return errors.WithStack(err)
	}
	return nil
}

func ensureServiceAccount(client *ClusterClient, name string, labels *provision.LabelSet, namespace string) error {
	svcAccount := apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels.ToLabels(),
		},
	}
	_, err := client.CoreV1().ServiceAccounts(namespace).Create(&svcAccount)
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
	ns, err := client.AppNamespace(a)
	if err != nil {
		return err
	}
	return ensureServiceAccount(client, serviceAccountNameForApp(a), labels, ns)
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
	yamlData, err := image.GetImageTsuruYamlData(imageName)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	var hcData hcResult
	if process == webProcessName {
		hcData, err = probesFromHC(yamlData.Healthcheck, portInt)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	var lifecycle *apiv1.Lifecycle
	if len(yamlData.Hooks.Restart.After) > 0 {
		hookCmds := []string{
			"sh", "-c",
			strings.Join(yamlData.Hooks.Restart.After, " && "),
		}
		lifecycle = &apiv1.Lifecycle{
			PostStart: &apiv1.Handler{
				Exec: &apiv1.ExecAction{
					Command: hookCmds,
				},
			},
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
	ns, err := client.AppNamespace(a)
	if err != nil {
		return nil, nil, nil, err
	}
	pullSecrets, err := getImagePullSecrets(client, ns, imageName)
	if err != nil {
		return nil, nil, nil, err
	}
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	expandedLabels := labels.ToLabels()
	expandedLabelsNoReplicas := labels.WithoutAppReplicas().ToLabels()
	rawAppLabel := appLabelForApp(a, process)
	expandedLabels["app"] = rawAppLabel
	expandedLabelsNoReplicas["app"] = rawAppLabel
	_, tag := image.SplitImageName(imageName)
	expandedLabels["version"] = tag
	expandedLabelsNoReplicas["version"] = tag
	var gracePeriod *int64
	policyLocal, err := client.ExternalPolicyLocal(a.GetPool())
	if err != nil {
		return nil, nil, nil, err
	}
	if policyLocal {
		// Grace period must be larger to account for the the pre-stop hook call
		gracePeriod = &defaultGracePeriodSeconds
	}
	deployment := v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   ns,
			Labels:      expandedLabels,
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
					Labels:      expandedLabelsNoReplicas,
					Annotations: annotations.ToLabels(),
				},
				Spec: apiv1.PodSpec{
					TerminationGracePeriodSeconds: gracePeriod,
					ImagePullSecrets:              pullSecrets,
					ServiceAccountName:            serviceAccountNameForApp(a),
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
							ReadinessProbe: hcData.readiness,
							LivenessProbe:  hcData.liveness,
							Resources: apiv1.ResourceRequirements{
								Limits:   resourceLimits,
								Requests: resourceRequests,
							},
							VolumeMounts: mounts,
							Ports: []apiv1.ContainerPort{
								{ContainerPort: int32(portInt)},
							},
							Lifecycle: lifecycle,
						},
					},
				},
			},
		},
	}
	var newDep *v1beta2.Deployment
	if oldDeployment == nil {
		newDep, err = client.AppsV1beta2().Deployments(ns).Create(&deployment)
	} else {
		newDep, err = client.AppsV1beta2().Deployments(ns).Update(&deployment)
	}
	return newDep, labels, annotations, errors.WithStack(err)
}

type serviceManager struct {
	client *ClusterClient
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) RemoveService(a provision.App, process string) error {
	ns, err := m.client.AppNamespace(a)
	if err != nil {
		return err
	}
	multiErrors := tsuruErrors.NewMultiError()
	err = cleanupDeployment(m.client, a, process)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	depName := deploymentNameForApp(a, process)
	err = m.client.CoreV1().Services(ns).Delete(depName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	headlessSvcName := headlessServiceNameForApp(a, process)
	err = m.client.CoreV1().Services(ns).Delete(headlessSvcName, &metav1.DeleteOptions{
		PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
	})
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(errors.WithStack(err))
	}
	return multiErrors.ToError()
}

func (m *serviceManager) CurrentLabels(a provision.App, process string) (*provision.LabelSet, error) {
	depName := deploymentNameForApp(a, process)
	ns, err := m.client.AppNamespace(a)
	if err != nil {
		return nil, err
	}
	dep, err := m.client.AppsV1beta2().Deployments(ns).Get(depName, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return labelSetFromMeta(&dep.ObjectMeta), nil
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

func filteredPodEvents(client *ClusterClient, evtResourceVersion, podName, namespace string) (watch.Interface, error) {
	var err error
	client, err = NewClusterClient(client.Cluster)
	if err != nil {
		return nil, err
	}
	err = client.SetTimeout(time.Hour)
	if err != nil {
		return nil, err
	}
	opts := listOptsForPodEvent(podName)
	opts.Watch = true
	opts.ResourceVersion = evtResourceVersion
	evtWatch, err := client.CoreV1().Events(namespace).Watch(opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return evtWatch, nil
}

func listOptsForPodEvent(podName string) metav1.ListOptions {
	selector := map[string]string{
		"involvedObject.kind": "Pod",
	}
	if podName != "" {
		selector["involvedObject.name"] = podName
	}
	return metav1.ListOptions{
		FieldSelector: labels.SelectorFromSet(labels.Set(selector)).String(),
	}
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

func monitorDeployment(ctx context.Context, client *ClusterClient, dep *v1beta2.Deployment, a provision.App, processName string, w io.Writer, evtResourceVersion string) (string, error) {
	revision := dep.Annotations[replicaDepRevision]
	ns, err := client.AppNamespace(a)
	if err != nil {
		return revision, err
	}
	watch, err := filteredPodEvents(client, evtResourceVersion, "", ns)
	if err != nil {
		return revision, err
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
		dep, err = client.AppsV1beta2().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return revision, err
		}
		revision = dep.Annotations[replicaDepRevision]
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeout:
			return revision, errors.Errorf("timeout waiting for deployment generation to update")
		case <-ctx.Done():
			return revision, ctx.Err()
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
				return revision, errors.Errorf("deployment %q exceeded its progress deadline", dep.Name)
			}
		}
		if oldUpdatedReplicas != dep.Status.UpdatedReplicas {
			fmt.Fprintf(w, " ---> %d of %d new units created\n", dep.Status.UpdatedReplicas, specReplicas)
		}
		if healthcheckTimeout == nil && dep.Status.UpdatedReplicas == specReplicas {
			var allInit bool
			allInit, err = allNewPodsRunning(client, a, processName, revision)
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
			return revision, createDeployTimeoutError(client, a, processName, w, time.Since(t0), "healthcheck")
		case <-timeout:
			return revision, createDeployTimeoutError(client, a, processName, w, time.Since(t0), "full rollout")
		case <-ctx.Done():
			return revision, ctx.Err()
		}
		dep, err = client.AppsV1beta2().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return revision, err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return revision, nil
}

func (m *serviceManager) DeployService(ctx context.Context, a provision.App, process string, labels *provision.LabelSet, replicas int, img string) error {
	err := ensureNodeContainers(a)
	if err != nil {
		return err
	}
	err = ensureNamespaceForApp(m.client, a)
	if err != nil {
		return err
	}
	err = ensureServiceAccountForApp(m.client, a)
	if err != nil {
		return err
	}
	depName := deploymentNameForApp(a, process)
	ns, err := m.client.AppNamespace(a)
	if err != nil {
		return err
	}
	oldDep, err := m.client.AppsV1beta2().Deployments(ns).Get(depName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		oldDep = nil
	}
	var oldRevision string
	if oldDep != nil {
		oldRevision = oldDep.Annotations[replicaDepRevision]
	}
	events, err := m.client.CoreV1().Events(ns).List(listOptsForPodEvent(""))
	if err != nil {
		return errors.WithStack(err)
	}
	newDep, labels, annotations, err := createAppDeployment(m.client, oldDep, a, process, img, replicas, labels)
	if err != nil {
		return err
	}
	if m.writer == nil {
		m.writer = ioutil.Discard
	}
	newRevision, err := monitorDeployment(ctx, m.client, newDep, a, process, m.writer, events.ResourceVersion)
	if err != nil {
		// We should only rollback if the updated deployment is a new revision.
		var rollbackErr error
		if oldDep != nil && (newRevision == "" || oldRevision == newRevision) {
			oldDep.Generation = 0
			oldDep.ResourceVersion = ""
			fmt.Fprintf(m.writer, "\n**** UPDATING BACK AFTER FAILURE ****\n ---> %s <---\n", err)
			_, rollbackErr = m.client.AppsV1beta2().Deployments(ns).Update(oldDep)
		} else if oldDep == nil && newDep != nil {
			// We have just created the deployment, so we need to remove it
			fmt.Fprintf(m.writer, "\n**** DELETING CREATED DEPLOYMENT AFTER FAILURE ****\n ---> %s <---\n", err)
			rollbackErr = m.client.AppsV1beta2().Deployments(ns).Delete(newDep.Name, &metav1.DeleteOptions{})
		} else {
			fmt.Fprintf(m.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n ---> %s <---\n", err)
			rollbackErr = m.client.ExtensionsV1beta1().Deployments(ns).Rollback(&extensions.DeploymentRollback{
				Name: depName,
			})
		}
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		return provision.ErrUnitStartup{Err: err}
	}
	expandedLabels := labels.ToLabels()
	labels.SetIsHeadlessService()
	expandedLabelsHeadless := labels.ToLabels()
	rawAppLabel := appLabelForApp(a, process)
	expandedLabels["app"] = rawAppLabel
	expandedLabelsHeadless["app"] = rawAppLabel
	targetPort := getTargetPortForImage(img)
	port, _ := strconv.Atoi(provision.WebProcessDefaultPort())
	policyLocal, err := m.client.ExternalPolicyLocal(a.GetPool())
	if err != nil {
		return err
	}
	policy := apiv1.ServiceExternalTrafficPolicyTypeCluster
	if policyLocal {
		policy = apiv1.ServiceExternalTrafficPolicyTypeLocal
	}
	svc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   ns,
			Labels:      expandedLabels,
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels.ToSelector(),
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(port),
					TargetPort: intstr.FromInt(targetPort),
					Name:       "http-default",
				},
			},
			Type:                  apiv1.ServiceTypeNodePort,
			ExternalTrafficPolicy: policy,
		},
	}
	svc, isNew, err := mergeServices(m.client, svc)
	if err != nil {
		return err
	}
	if isNew {
		_, err = m.client.CoreV1().Services(ns).Create(svc)
	} else {
		_, err = m.client.CoreV1().Services(ns).Update(svc)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	kubeConf := getKubeConfig()
	headlessSvc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        headlessServiceNameForApp(a, process),
			Namespace:   ns,
			Labels:      expandedLabelsHeadless,
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels.ToSelector(),
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(kubeConf.HeadlessServicePort),
					TargetPort: intstr.FromInt(targetPort),
					Name:       "http-headless",
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	}
	headlessSvc, isNew, err = mergeServices(m.client, headlessSvc)
	if err != nil {
		return err
	}
	if isNew {
		_, err = m.client.CoreV1().Services(ns).Create(headlessSvc)
	} else {
		_, err = m.client.CoreV1().Services(ns).Update(headlessSvc)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func mergeServices(client *ClusterClient, svc *apiv1.Service) (*apiv1.Service, bool, error) {
	existing, err := client.CoreV1().Services(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return svc, true, nil
		}
		return nil, false, errors.WithStack(err)
	}
	for i := 0; i < len(svc.Spec.Ports) && i < len(existing.Spec.Ports); i++ {
		svc.Spec.Ports[i].NodePort = existing.Spec.Ports[i].NodePort
	}
	svc.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP
	return svc, false, nil
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

func imageTagAndPush(client *ClusterClient, a provision.App, oldImage, newImage string) (InspectData, error) {
	deployPodName, err := deployPodNameForApp(a)
	if err != nil {
		return InspectData{}, err
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
		return InspectData{}, err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	destImages := []string{newImage}
	repository, tag := image.SplitImageName(newImage)
	if tag != "latest" {
		destImages = append(destImages, fmt.Sprintf("%s:latest", repository))
	}
	err = runInspectSidecar(inspectParams{
		client:            client,
		stdout:            stdout,
		stderr:            stderr,
		app:               a,
		sourceImage:       oldImage,
		destinationImages: destImages,
		podName:           deployPodName,
		labels:            labels,
	})
	if err != nil {
		return InspectData{}, errors.Wrapf(err, "unable to pull and tag image: stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}
	var data InspectData
	bufData := stdout.String()
	err = json.NewDecoder(stdout).Decode(&data)
	if err != nil {
		return InspectData{}, errors.Wrapf(err, "invalid image inspect response: %q", bufData)
	}
	return data, err
}

type inspectParams struct {
	sourceImage       string
	podName           string
	destinationImages []string
	stdout            io.Writer
	stderr            io.Writer
	client            *ClusterClient
	labels            *provision.LabelSet
	app               provision.App
}

func runInspectSidecar(params inspectParams) error {
	inspectContainer := "inspect-cont"
	kubeConf := getKubeConfig()
	pod, err := newDeployAgentPod(params.client, params.sourceImage, params.app, params.podName, deployAgentConfig{
		name:              inspectContainer,
		image:             kubeConf.DeployInspectImage,
		cmd:               "cat >/dev/null && /bin/deploy-agent",
		destinationImages: params.destinationImages,
		sourceImage:       params.sourceImage,
	})
	if err != nil {
		return err
	}
	ns, err := params.client.AppNamespace(params.app)
	if err != nil {
		return err
	}
	_, err = params.client.CoreV1().Pods(ns).Create(&pod)
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(params.client, pod.Name, ns)
	multiErr := tsuruErrors.NewMultiError()
	ctx, cancel := context.WithTimeout(context.Background(), kubeConf.PodRunningTimeout)
	err = waitForPodContainersRunning(ctx, params.client, &pod, ns)
	cancel()
	if err != nil {
		multiErr.Add(errors.WithStack(err))
	}
	err = doAttach(context.TODO(), params.client, bytes.NewBufferString("."), params.stdout, params.stderr, pod.Name, inspectContainer, false, nil, ns)
	if err != nil {
		multiErr.Add(errors.WithStack(err))
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	ctx, cancel = context.WithTimeout(context.Background(), kubeConf.PodRunningTimeout)
	defer cancel()
	return waitForPod(ctx, params.client, &pod, ns, false)
}

type deployAgentConfig struct {
	name              string
	image             string
	cmd               string
	destinationImages []string
	sourceImage       string
	inputFile         string
	registryAuthPass  string
	registryAuthUser  string
	registryAddress   string
	runAsUser         string
	dockerfileBuild   bool
}

func newDeployAgentPod(client *ClusterClient, sourceImage string, app provision.App, podName string, conf deployAgentConfig) (apiv1.Pod, error) {
	if len(conf.destinationImages) == 0 {
		return apiv1.Pod{}, errors.Errorf("no destination images provided")
	}
	err := ensureNamespaceForApp(client, app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	err = ensureServiceAccountForApp(client, app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return apiv1.Pod{}, err
	}
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	volumes, mounts, err := createVolumesForApp(client, app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	annotations.SetBuildImage(conf.destinationImages[0])
	appEnvs := provision.EnvsForApp(app, "", true)
	var envs []apiv1.EnvVar
	for _, envData := range appEnvs {
		envs = append(envs, apiv1.EnvVar{Name: envData.Name, Value: envData.Value})
	}
	nodeSelector := provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   app.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector()
	_, uid := dockercommon.UserForContainer()
	ns, err := client.AppNamespace(app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	pullSecrets, err := getImagePullSecrets(client, ns, sourceImage, conf.image)
	if err != nil {
		return apiv1.Pod{}, err
	}
	if uid != nil && conf.runAsUser == "" {
		conf.runAsUser = strconv.FormatInt(*uid, 10)
	}
	return apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			ImagePullSecrets:   pullSecrets,
			ServiceAccountName: serviceAccountNameForApp(app),
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
				newSleepyContainer(podName, sourceImage, uid, envs, mounts...),
				newDeployAgentContainer(conf),
			},
		},
	}, nil
}

func newDeployAgentImageBuildPod(client *ClusterClient, sourceImage string, podName string, conf deployAgentConfig) (apiv1.Pod, error) {
	if len(conf.destinationImages) == 0 {
		return apiv1.Pod{}, errors.Errorf("no destination images provided")
	}
	err := ensureNamespaceForApp(client, nil)
	if err != nil {
		return apiv1.Pod{}, err
	}
	labels := provision.ImageBuildLabels(provision.ImageBuildLabelsOpts{
		IsBuild:     true,
		Prefix:      tsuruLabelPrefix,
		Provisioner: provisionerName,
	})
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	annotations.SetBuildImage(conf.destinationImages[0])
	nodePoolLabelKey := tsuruLabelPrefix + provision.LabelNodePool
	selectors := []apiv1.NodeSelectorRequirement{
		{Key: nodePoolLabelKey, Operator: apiv1.NodeSelectorOpExists},
	}
	affinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: selectors,
				}},
			},
		},
	}
	_, uid := dockercommon.UserForContainer()
	ns := client.Namespace()
	pullSecrets, err := getImagePullSecrets(client, ns, sourceImage, conf.image)
	if err != nil {
		return apiv1.Pod{}, err
	}
	if uid != nil && conf.runAsUser == "" {
		conf.runAsUser = strconv.FormatInt(*uid, 10)
	}
	conf.registryAuthUser, conf.registryAuthPass, conf.registryAddress = registryAuth(conf.destinationImages[0])
	return apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			Affinity:         affinity,
			ImagePullSecrets: pullSecrets,
			Volumes: []apiv1.Volume{
				{
					Name: "dockersock",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: dockerSockPath,
						},
					},
				},
			},
			RestartPolicy: apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:  conf.name,
					Image: conf.image,
					VolumeMounts: append([]apiv1.VolumeMount{
						{Name: "dockersock", MountPath: dockerSockPath},
					}),
					Stdin:     true,
					StdinOnce: true,
					Env:       conf.asEnvs(),
					Command: []string{
						"sh", "-ec",
						fmt.Sprintf(`
								%[1]s
							`, conf.cmd),
					},
				},
			},
		},
	}, nil
}

func (c deployAgentConfig) asEnvs() []apiv1.EnvVar {
	return []apiv1.EnvVar{
		{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
		{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: strings.Join(c.destinationImages, ",")},
		{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: c.sourceImage},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: c.registryAuthUser},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: c.registryAuthPass},
		{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: c.registryAddress},
		{Name: "DEPLOYAGENT_INPUT_FILE", Value: c.inputFile},
		{Name: "DEPLOYAGENT_RUN_AS_USER", Value: c.runAsUser},
		{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: strconv.FormatBool(c.dockerfileBuild)},
	}
}

func newDeployAgentContainer(conf deployAgentConfig) apiv1.Container {
	conf.registryAuthUser, conf.registryAuthPass, conf.registryAddress = registryAuth(conf.destinationImages[0])
	return apiv1.Container{
		Name:  conf.name,
		Image: conf.image,
		VolumeMounts: append([]apiv1.VolumeMount{
			{Name: "dockersock", MountPath: dockerSockPath},
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		}),
		Stdin:     true,
		StdinOnce: true,
		Env:       conf.asEnvs(),
		Command: []string{
			"sh", "-ec",
			fmt.Sprintf(`
				end() { touch %[1]s; }
				trap end EXIT
				%[2]s
			`, buildIntercontainerDone, conf.cmd),
		},
	}
}

func newSleepyContainer(name, image string, uid *int64, envs []apiv1.EnvVar, mounts ...apiv1.VolumeMount) apiv1.Container {
	return apiv1.Container{
		Name:  name,
		Image: image,
		Env:   envs,
		SecurityContext: &apiv1.SecurityContext{
			RunAsUser: uid,
		},
		Command: []string{"/bin/sh", "-ec", fmt.Sprintf("while [ ! -f %s ]; do sleep 5; done", buildIntercontainerDone)},
		VolumeMounts: append([]apiv1.VolumeMount{
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		}, mounts...),
	}
}
