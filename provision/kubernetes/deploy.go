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
	"sort"
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
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	appsv1 "k8s.io/api/apps/v1"
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
	defaultHttpPortName     = "http-default"
	defaultUdpPortName      = "udp-default"
)

type InspectData struct {
	Image     docker.Image
	TsuruYaml provTypes.TsuruYamlData
	Procfile  string
}

func keepAliveSpdyExecutor(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}
	upgradeRoundTripper := spdy.NewSpdyRoundTripper(tlsConfig, true, false)
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
	params.cmds = cmds
	return createPod(ctx, params)
}

func createDeployPod(ctx context.Context, params createPodParams) error {
	if len(params.destinationImages) == 0 {
		return fmt.Errorf("no destination images provided")
	}
	cmds := dockercommon.DeployCmds(params.app)
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
	closeFn, err := logPodEvents(params.client, events.ResourceVersion, params.podName, ns, params.attachOutput)
	if err != nil {
		return err
	}
	defer closeFn()
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

func tsuruHostToken(a provision.App) (string, string) {
	host, _ := config.GetString("host")
	if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	if !strings.HasSuffix(host, "/") {
		host += "/"
	}
	token := a.Envs()["TSURU_APP_TOKEN"].Value
	return host, token
}

func extraRegisterCmds(a provision.App) string {
	host, token := tsuruHostToken(a)
	return fmt.Sprintf(`curl -sSL -m15 -XPOST -d"hostname=$(hostname)" -o/dev/null -H"Content-Type:application/x-www-form-urlencoded" -H"Authorization:bearer %s" %sapps/%s/units/register || true`, token, host, a.GetName())
}

func logPodEvents(client *ClusterClient, initialResourceVersion, podName, namespace string, output io.Writer) (func(), error) {
	watch, err := filteredPodEvents(client, initialResourceVersion, podName, namespace)
	if err != nil {
		return nil, err
	}
	watchDone := make(chan struct{})
	watchCh := watch.ResultChan()
	go func() {
		defer close(watchDone)
		for {
			msg, isOpen := <-watchCh
			if !isOpen {
				return
			}
			fmt.Fprintf(output, " ---> %s\n", formatEvtMessage(msg, true))
		}
	}()
	return func() {
		watch.Stop()
		<-watchDone
	}, nil
}

type hcResult struct {
	liveness  *apiv1.Probe
	readiness *apiv1.Probe
}

func probesFromHC(hc *provTypes.TsuruYamlHealthcheck, port int) (hcResult, error) {
	var result hcResult
	if hc == nil || (hc.Path == "" && len(hc.Command) == 0) {
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
	if hc.AllowedFailures == 0 {
		hc.AllowedFailures = 3
	}
	if hc.Method != http.MethodGet {
		return result, errors.New("healthcheck: only GET method is supported in kubernetes provisioner with use_in_router set")
	}
	headers := []apiv1.HTTPHeader{}
	for header, value := range hc.Headers {
		headers = append(headers, apiv1.HTTPHeader{Name: header, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	hc.Scheme = strings.ToUpper(hc.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: int32(hc.AllowedFailures),
		PeriodSeconds:    int32(hc.IntervalSeconds),
		TimeoutSeconds:   int32(hc.TimeoutSeconds),
		Handler:          apiv1.Handler{},
	}
	if hc.Path != "" {
		probe.Handler.HTTPGet = &apiv1.HTTPGetAction{
			Path:        hc.Path,
			Port:        intstr.FromInt(port),
			Scheme:      apiv1.URIScheme(hc.Scheme),
			HTTPHeaders: headers,
		}
	} else {
		probe.Handler.Exec = &apiv1.ExecAction{
			Command: hc.Command,
		}
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

func createAppDeployment(client *ClusterClient, depName string, oldDeployment *appsv1.Deployment, a provision.App, process string, version appTypes.AppVersion, replicas int, labels *provision.LabelSet, selector map[string]string) (*appsv1.Deployment, *provision.LabelSet, *provision.LabelSet, error) {
	realReplicas := int32(replicas)
	extra := []string{extraRegisterCmds(a)}
	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, cmdData, a, extra)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	tenRevs := int32(10)
	webProcessName, err := version.WebProcess()
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	yamlData, err := version.TsuruYamlData()
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	processPorts, err := getProcessPortsForVersion(version, process)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	var hcData hcResult
	if process == webProcessName && len(processPorts) > 0 {
		//TODO: add support to multiple HCs
		hcData, err = probesFromHC(yamlData.Healthcheck, processPorts[0].TargetPort)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	var lifecycle *apiv1.Lifecycle
	if yamlData.Hooks != nil && len(yamlData.Hooks.Restart.After) > 0 {
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
	maxSurge := client.maxSurge(a.GetPool())
	maxUnavailable := client.maxUnavailable(a.GetPool())
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
	deployImage := version.VersionInfo().DeployImage
	pullSecrets, err := getImagePullSecrets(client, ns, deployImage)
	if err != nil {
		return nil, nil, nil, err
	}
	labels, annotations := provision.SplitServiceLabelsAnnotations(labels)
	depLabels := labels.WithoutVersion().ToLabels()
	podLabels := labels.WithoutAppReplicas().ToLabels()
	baseName := deploymentNameForAppBase(a, process)
	depLabels["app"] = baseName
	podLabels["app"] = baseName
	containerPorts := make([]apiv1.ContainerPort, len(processPorts))
	for i, port := range processPorts {
		portInt := port.TargetPort
		if portInt == 0 {
			portInt, _ = strconv.Atoi(provision.WebProcessDefaultPort())
		}
		containerPorts[i].ContainerPort = int32(portInt)
	}
	serviceLinks := false

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   ns,
			Labels:      depLabels,
			Annotations: annotations.ToLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &realReplicas,
			RevisionHistoryLimit: &tenRevs,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: annotations.ToLabels(),
				},
				Spec: apiv1.PodSpec{
					EnableServiceLinks: &serviceLinks,
					ImagePullSecrets:   pullSecrets,
					ServiceAccountName: serviceAccountNameForApp(a),
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: uid,
					},
					RestartPolicy: apiv1.RestartPolicyAlways,
					NodeSelector:  nodeSelector,
					Volumes:       volumes,
					Subdomain:     headlessServiceName(a, process),
					Containers: []apiv1.Container{
						{
							Name:           depName,
							Image:          deployImage,
							Command:        cmds,
							Env:            appEnvs(a, process, version, false),
							ReadinessProbe: hcData.readiness,
							LivenessProbe:  hcData.liveness,
							Resources: apiv1.ResourceRequirements{
								Limits:   resourceLimits,
								Requests: resourceRequests,
							},
							VolumeMounts: mounts,
							Ports:        containerPorts,
							Lifecycle:    lifecycle,
						},
					},
				},
			},
		},
	}
	var newDep *appsv1.Deployment
	if oldDeployment == nil {
		newDep, err = client.AppsV1().Deployments(ns).Create(&deployment)
	} else {
		newDep, err = client.AppsV1().Deployments(ns).Update(&deployment)
	}
	return newDep, labels, annotations, errors.WithStack(err)
}

func appEnvs(a provision.App, process string, version appTypes.AppVersion, isDeploy bool) []apiv1.EnvVar {
	appEnvs := EnvsForApp(a, process, version, isDeploy)
	envs := make([]apiv1.EnvVar, len(appEnvs))
	for i, envData := range appEnvs {
		envs[i] = apiv1.EnvVar{Name: envData.Name, Value: envData.Value}
	}
	return envs
}

type serviceManager struct {
	client *ClusterClient
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) CleanupServices(a provision.App, deployedVersion appTypes.AppVersion, preserveOldVersions bool) error {
	depGroups, err := deploymentsDataForApp(m.client, a)
	if err != nil {
		return err
	}

	type processVersionKey struct {
		process string
		version int
	}

	fmt.Fprint(m.writer, "\n---- Cleaning up resources ----\n")

	processInUse := map[string]struct{}{}
	versionInUse := map[processVersionKey]struct{}{}
	multiErrors := tsuruErrors.NewMultiError()
	for _, depsData := range depGroups.versioned {
		for _, depData := range depsData {
			toKeep := depData.replicas > 0 && (preserveOldVersions || depData.version == deployedVersion.Version())

			if toKeep {
				processInUse[depData.process] = struct{}{}
				versionInUse[processVersionKey{process: depData.process, version: depData.version}] = struct{}{}
				continue
			}

			fmt.Fprintf(m.writer, " ---> Cleaning up deployment %s\n", depData.dep)
			err = cleanupSingleDeployment(m.client, depData.dep)
			if err != nil {
				multiErrors.Add(err)
			}
		}
	}

	svcs, err := allServicesForApp(m.client, a)
	if err != nil {
		multiErrors.Add(err)
	}
	for _, svc := range svcs {
		labels := labelSetFromMeta(&svc.ObjectMeta)
		svcVersion := labels.AppVersion()
		_, inUseProcess := processInUse[labels.AppProcess()]
		_, inUseVersion := versionInUse[processVersionKey{process: labels.AppProcess(), version: svcVersion}]

		toKeep := inUseVersion || (svcVersion == 0 && inUseProcess)

		if toKeep {
			continue
		}

		fmt.Fprintf(m.writer, " ---> Cleaning up service %s\n", svc.Name)
		err = m.client.CoreV1().Services(svc.Namespace).Delete(svc.Name, &metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			multiErrors.Add(err)
		}
	}

	return multiErrors.ToError()
}

func (m *serviceManager) RemoveService(a provision.App, process string, version appTypes.AppVersion) error {
	multiErrors := tsuruErrors.NewMultiError()
	err := cleanupDeployment(m.client, a, process, version)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	err = cleanupServices(m.client, a, process, version)
	if err != nil {
		multiErrors.Add(err)
	}
	return multiErrors.ToError()
}

func (m *serviceManager) CurrentLabels(a provision.App, process string, version appTypes.AppVersion) (*provision.LabelSet, error) {
	dep, err := deploymentForVersion(m.client, a, process, version)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return labelSetFromMeta(&dep.ObjectMeta), nil
}

const deadlineExeceededProgressCond = "ProgressDeadlineExceeded"

func createDeployTimeoutError(client *ClusterClient, ns string, selector map[string]string, w io.Writer, timeout time.Duration, label string) error {
	messages, err := notReadyPodEvents(client, ns, selector)
	if err != nil {
		return errors.Wrap(err, "Unknown error deploying application")
	}
	if len(messages) == 0 {
		// This should not be possible.
		return errors.Errorf("Unknown error deploying application, timeout after %v", timeout)
	}
	var msgsStr []string
	badUnitsSet := make(map[string]struct{})
	for _, m := range messages {
		badUnitsSet[m.podName] = struct{}{}
		msgsStr = append(msgsStr, fmt.Sprintf(" ---> %s", m.message))
	}
	var badUnits []string
	for u := range badUnitsSet {
		badUnits = append(badUnits, u)
	}
	return provision.ErrUnitStartup{
		BadUnits: badUnits,
		Err:      errors.New(strings.Join(msgsStr, "\n")),
	}
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

func isDeploymentEvent(msg watch.Event, dep *appsv1.Deployment) bool {
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

func monitorDeployment(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment, a provision.App, processName string, w io.Writer, evtResourceVersion string, version appTypes.AppVersion) (string, error) {
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
	fmt.Fprintf(w, "\n---- Updating units [%s] [version %d] ----\n", processName, version.Version())
	kubeConf := getKubeConfig()
	timer := time.NewTimer(kubeConf.DeploymentProgressTimeout)
	for dep.Status.ObservedGeneration < dep.Generation {
		dep, err = client.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return revision, err
		}
		revision = dep.Annotations[replicaDepRevision]
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timer.C:
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
	largestReady := int32(0)
	for {
		for i := range dep.Status.Conditions {
			c := dep.Status.Conditions[i]
			if c.Type == appsv1.DeploymentProgressing && c.Reason == deadlineExeceededProgressCond {
				return revision, errors.Errorf("deployment %q exceeded its progress deadline", dep.Name)
			}
		}
		if oldUpdatedReplicas != dep.Status.UpdatedReplicas {
			fmt.Fprintf(w, " ---> %d of %d new units created\n", dep.Status.UpdatedReplicas, specReplicas)
		}
		if healthcheckTimeout == nil && dep.Status.UpdatedReplicas == specReplicas {
			var allInit bool
			allInit, err = allNewPodsRunning(client, a, processName, dep, version)
			if allInit && err == nil {
				healthcheckTimeout = time.After(maxWaitTimeDuration)
				fmt.Fprintf(w, " ---> waiting healthcheck on %d created units\n", specReplicas)
			}
		}
		readyUnits := dep.Status.UpdatedReplicas - dep.Status.UnavailableReplicas
		if oldReadyUnits != readyUnits && readyUnits >= 0 {
			fmt.Fprintf(w, " ---> %d of %d new units ready\n", readyUnits, specReplicas)
		}
		if readyUnits > largestReady {
			largestReady = readyUnits
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(kubeConf.DeploymentProgressTimeout)
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
			return revision, createDeployTimeoutError(client, ns, dep.Spec.Selector.MatchLabels, w, time.Since(t0), "healthcheck")
		case <-timer.C:
			return revision, createDeployTimeoutError(client, ns, dep.Spec.Selector.MatchLabels, w, time.Since(t0), "full rollout")
		case <-ctx.Done():
			err = ctx.Err()
			if err == context.Canceled {
				err = errors.Wrap(err, "canceled by user action")
			}
			return revision, err
		}
		dep, err = client.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
		if err != nil {
			return revision, err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return revision, nil
}

func (m *serviceManager) DeployService(ctx context.Context, a provision.App, process string, labels *provision.LabelSet, replicas int, version appTypes.AppVersion, preserveVersions bool) error {
	if m.writer == nil {
		m.writer = ioutil.Discard
	}

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
	ns, err := m.client.AppNamespace(a)
	if err != nil {
		return err
	}

	provision.ExtendServiceLabels(labels, provision.ServiceLabelExtendedOpts{
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})

	depArgs, err := m.baseDeploymentArgs(a, process, labels, version, preserveVersions)
	if err != nil {
		return err
	}

	if depArgs.baseDep != nil && depArgs.baseDep.isLegacy && depArgs.name != depArgs.baseDep.dep.Name {
		fmt.Fprint(m.writer, "\n---- Updating legacy deployment ----\n")
		err = toggleRoutableDeployment(m.client, depArgs.baseDep.version, depArgs.baseDep.dep, true)
		if err != nil {
			return errors.Wrap(err, "unable to update legacy deployment")
		}
	}

	oldDep, err := m.client.AppsV1().Deployments(ns).Get(depArgs.name, metav1.GetOptions{})
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
	newDep, labels, annotations, err := createAppDeployment(m.client, depArgs.name, oldDep, a, process, version, replicas, labels, depArgs.selector)
	if err != nil {
		return err
	}
	newRevision, err := monitorDeployment(ctx, m.client, newDep, a, process, m.writer, events.ResourceVersion, version)
	if err != nil {
		// We should only rollback if the updated deployment is a new revision.
		var rollbackErr error
		if oldDep != nil && (newRevision == "" || oldRevision == newRevision) {
			oldDep.Generation = 0
			oldDep.ResourceVersion = ""
			fmt.Fprintf(m.writer, "\n**** UPDATING BACK AFTER FAILURE ****\n")
			_, rollbackErr = m.client.AppsV1().Deployments(ns).Update(oldDep)
		} else if oldDep == nil {
			// We have just created the deployment, so we need to remove it
			fmt.Fprintf(m.writer, "\n**** DELETING CREATED DEPLOYMENT AFTER FAILURE ****\n")
			rollbackErr = m.client.AppsV1().Deployments(ns).Delete(newDep.Name, &metav1.DeleteOptions{})
		} else {
			fmt.Fprintf(m.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n")
			rollbackErr = m.client.ExtensionsV1beta1().Deployments(ns).Rollback(&extensions.DeploymentRollback{
				Name: depArgs.name,
			})
		}
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		if _, ok := err.(provision.ErrUnitStartup); ok {
			return err
		}
		return provision.ErrUnitStartup{Err: err}
	}

	fmt.Fprintf(m.writer, "\n---- Ensuring services [%s] ----\n", process)
	err = m.ensureServices(a, process, labels, annotations)
	if err != nil {
		return err
	}
	return nil
}

type baseDepArgs struct {
	name     string
	selector map[string]string
	isLegacy bool
	baseDep  *deploymentInfo
}

func (m *serviceManager) baseDeploymentArgs(a provision.App, process string, labels *provision.LabelSet, version appTypes.AppVersion, preserveVersions bool) (baseDepArgs, error) {
	var result baseDepArgs
	if !preserveVersions {
		labels.SetIsRoutable()
		result.name = deploymentNameForAppBase(a, process)
		result.selector = labels.ToBaseSelector()
		return result, nil
	}

	depData, err := deploymentsDataForProcess(m.client, a, process)
	if err != nil {
		return result, err
	}

	result.baseDep = &depData.base

	if versionDeps, ok := depData.versioned[version.Version()]; ok {
		if len(versionDeps) != 1 {
			var names []string
			for _, vd := range versionDeps {
				names = append(names, vd.dep.Name)
			}
			return result, errors.Errorf("more than one deployment for the same app version found: %v", names)
		}
		versionDep := versionDeps[0]
		if versionDep.isRoutable {
			labels.SetIsRoutable()
		}
		if !versionDep.isBase {
			labels.ReplaceIsIsolatedRunWithNew()
		}
		result.isLegacy = versionDep.isLegacy
		result.name = versionDep.dep.Name
		result.selector = versionDep.dep.Spec.Selector.MatchLabels
		return result, nil
	}

	if depData.base.dep != nil {
		labels.ReplaceIsIsolatedRunWithNew()
		result.name = deploymentNameForApp(a, process, version.Version())
		result.selector = labels.ToVersionSelector()
		return result, nil
	}

	if depData.count == 0 {
		labels.SetIsRoutable()
	}
	result.name = deploymentNameForAppBase(a, process)
	result.selector = labels.ToBaseSelector()
	return result, nil
}

type svcCreateData struct {
	name     string
	labels   map[string]string
	selector map[string]string
	ports    []apiv1.ServicePort
}

func (m *serviceManager) ensureServices(a provision.App, process string, labels, annotations *provision.LabelSet) error {
	ns, err := m.client.AppNamespace(a)
	if err != nil {
		return err
	}

	policyLocal, err := m.client.ExternalPolicyLocal(a.GetPool())
	if err != nil {
		return err
	}
	policy := apiv1.ServiceExternalTrafficPolicyTypeCluster
	if policyLocal {
		policy = apiv1.ServiceExternalTrafficPolicyTypeLocal
	}

	labels = labels.WithoutAppReplicas()

	routableLabels := labels.WithoutVersion().WithoutIsolated()
	routableLabels.SetIsRoutable()

	versionLabels := labels.WithoutRoutable()

	depData, err := deploymentsDataForProcess(m.client, a, process)
	if err != nil {
		return err
	}

	versions, err := servicemanager.AppVersion.AppVersions(a)
	if err != nil {
		return err
	}
	versionInfoMap := map[int]appTypes.AppVersionInfo{}
	for _, v := range versions.Versions {
		versionInfoMap[v.Version] = v
	}

	var baseSvcPorts []apiv1.ServicePort
	var svcsToCreate []svcCreateData

	for versionNumber, depInfo := range depData.versioned {
		if len(depInfo) == 0 {
			continue
		}

		vInfo, ok := versionInfoMap[versionNumber]
		if !ok {
			return errors.Errorf("no version data found for %v", versionNumber)
		}
		version := servicemanager.AppVersion.AppVersionFromInfo(a, vInfo)
		svcPorts, err := loadServicePorts(version, process)
		if err != nil {
			return err
		}

		if len(svcPorts) == 0 {
			err = cleanupServices(m.client, a, process, version)
			if err != nil {
				return err
			}
			continue
		}

		labels := versionLabels.DeepCopy()
		labels.SetVersion(versionNumber)
		if depInfo[0].isBase {
			labels.ReplaceIsIsolatedNewRunWithBase()
		} else {
			labels.ReplaceIsIsolatedRunWithNew()
		}

		svcsToCreate = append(svcsToCreate, svcCreateData{
			name:     serviceNameForApp(a, process, versionNumber),
			labels:   labels.ToLabels(),
			selector: labels.ToVersionSelector(),
			ports:    svcPorts,
		})

		if depInfo[0].isRoutable {
			baseSvcPorts = deepCopyPorts(svcPorts)
		}
	}

	if baseSvcPorts != nil {
		svcsToCreate = append(svcsToCreate, svcCreateData{
			name:     serviceNameForAppBase(a, process),
			labels:   routableLabels.ToLabels(),
			selector: routableLabels.ToRoutableSelector(),
			ports:    baseSvcPorts,
		})
	}

	if len(svcsToCreate) == 0 {
		return nil
	}

	headlessPorts := deepCopyPorts(baseSvcPorts)

	for _, svcData := range svcsToCreate {
		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcData.name,
				Namespace:   ns,
				Labels:      svcData.labels,
				Annotations: annotations.ToLabels(),
			},
			Spec: apiv1.ServiceSpec{
				Selector:              svcData.selector,
				Ports:                 svcData.ports,
				Type:                  apiv1.ServiceTypeNodePort,
				ExternalTrafficPolicy: policy,
			},
		}
		var isNew bool
		svc, isNew, err = mergeServices(m.client, svc)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.writer, " ---> Service %s\n", svc.Name)
		if isNew {
			_, err = m.client.CoreV1().Services(svc.Namespace).Create(svc)
		} else {
			_, err = m.client.CoreV1().Services(svc.Namespace).Update(svc)
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err = m.createHeadlessService(headlessPorts, ns, a, process, routableLabels.WithoutVersion(), annotations)
	if err != nil {
		return err
	}
	return nil
}

func (m *serviceManager) createHeadlessService(svcPorts []apiv1.ServicePort, ns string, a provision.App, process string, labels, annotations *provision.LabelSet) error {
	enabled, err := m.client.headlessEnabled(a.GetPool())
	if err != nil {
		return errors.WithStack(err)
	}
	if !enabled {
		return nil
	}
	svcName := headlessServiceName(a, process)
	fmt.Fprintf(m.writer, " ---> Service %s\n", svcName)

	labels.SetIsHeadlessService()
	expandedLabelsHeadless := labels.ToLabels()

	headlessPorts := []apiv1.ServicePort{}
	for i, svcPort := range svcPorts {
		headlessPorts = append(headlessPorts, apiv1.ServicePort{
			Name:       fmt.Sprintf("http-headless-%d", i+1),
			Protocol:   svcPort.Protocol,
			Port:       svcPort.Port,
			TargetPort: svcPort.TargetPort,
		})
	}
	if len(headlessPorts) == 0 {
		kubeConf := getKubeConfig()
		headlessPorts = append(headlessPorts, apiv1.ServicePort{
			Name:       "http-headless-1",
			Protocol:   apiv1.ProtocolTCP,
			Port:       int32(kubeConf.HeadlessServicePort),
			TargetPort: intstr.FromInt(kubeConf.HeadlessServicePort),
		})
	}

	headlessSvc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Namespace:   ns,
			Labels:      expandedLabelsHeadless,
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.ServiceSpec{
			Selector:  labels.ToRoutableSelector(),
			Ports:     headlessPorts,
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	}
	headlessSvc, isNew, err := mergeServices(m.client, headlessSvc)
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

func loadServicePorts(version appTypes.AppVersion, processName string) ([]apiv1.ServicePort, error) {
	processPorts, err := getProcessPortsForVersion(version, processName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	svcPorts := make([]apiv1.ServicePort, len(processPorts))
	defaultPort, _ := strconv.Atoi(provision.WebProcessDefaultPort())
	for i, port := range processPorts {
		svcPorts[i].Protocol = apiv1.Protocol(port.Protocol)
		if port.TargetPort > 0 {
			svcPorts[i].TargetPort = intstr.FromInt(port.TargetPort)
		} else {
			svcPorts[i].TargetPort = intstr.FromInt(defaultPort)
		}
		if port.Port > 0 {
			svcPorts[i].Port = int32(port.Port)
		} else {
			svcPorts[i].Port = int32(defaultPort)
		}
		svcPorts[i].Name = port.Name
	}
	return svcPorts, nil
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

func getTargetPortsForVersion(version appTypes.AppVersion) []string {
	exposedPorts := version.VersionInfo().ExposedPorts
	if len(exposedPorts) > 0 {
		return exposedPorts
	}
	return []string{provision.WebProcessDefaultPort() + "/tcp"}
}

func extractPortNumberAndProtocol(port string) (int, string, error) {
	parts := strings.SplitN(port, "/", 2)
	if len(parts) != 2 {
		return 0, "", errors.New("invalid port: " + port)
	}
	portInt, err := strconv.Atoi(parts[0])
	return portInt, parts[1], err
}

func getProcessPortsForVersion(version appTypes.AppVersion, process string) ([]provTypes.TsuruYamlKubernetesProcessPortConfig, error) {
	portConfigFound := false
	var ports []provTypes.TsuruYamlKubernetesProcessPortConfig
	tsuruYamlData, err := version.TsuruYamlData()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if tsuruYamlData.Kubernetes != nil {
		for _, group := range tsuruYamlData.Kubernetes.Groups {
			for podName, podConfig := range group {
				if podName == process {
					if portConfigFound {
						return nil, fmt.Errorf("duplicated process name: %s", podName)
					}
					portConfigFound = true
					ports = podConfig.Ports
					for i := range ports {
						if len(ports[i].Protocol) == 0 {
							ports[i].Protocol = string(apiv1.ProtocolTCP)
						} else {
							ports[i].Protocol = strings.ToUpper(ports[i].Protocol)
						}
						if len(ports[i].Name) == 0 {
							prefix := defaultHttpPortName
							if ports[i].Protocol == string(apiv1.ProtocolUDP) {
								prefix = defaultUdpPortName
							}
							ports[i].Name = fmt.Sprintf("%s-%d", prefix, i+1)
						}
						if ports[i].TargetPort > 0 && ports[i].Port == 0 {
							ports[i].Port = ports[i].TargetPort
						} else if ports[i].Port > 0 && ports[i].TargetPort == 0 {
							ports[i].TargetPort = ports[i].Port
						}
					}
					break
				}
			}
		}
	}
	if portConfigFound {
		return ports, nil
	}

	defaultPort := defaultKubernetesPodPortConfig()
	targetPorts := getTargetPortsForVersion(version)
	ports = make([]provTypes.TsuruYamlKubernetesProcessPortConfig, len(targetPorts))
	for i := range ports {
		portInt, protocol, err := extractPortNumberAndProtocol(targetPorts[i])
		if err != nil {
			continue
		}
		ports[i].Protocol = strings.ToUpper(protocol)
		prefix := defaultPort.Name
		if ports[i].Protocol == string(apiv1.ProtocolUDP) {
			prefix = defaultUdpPortName
		}
		ports[i].Name = fmt.Sprintf("%s-%d", prefix, i+1)
		if len(targetPorts) == 1 {
			ports[i].Port = defaultPort.Port
		} else {
			ports[i].Port = portInt
		}
		ports[i].TargetPort = portInt
	}
	if len(ports) == 0 {
		ports = append(ports, defaultPort)
	}
	return ports, nil
}

func defaultKubernetesPodPortConfig() provTypes.TsuruYamlKubernetesProcessPortConfig {
	defaultPort, _ := strconv.Atoi(provision.WebProcessDefaultPort())
	return provTypes.TsuruYamlKubernetesProcessPortConfig{
		Name:       defaultHttpPortName,
		Protocol:   string(apiv1.ProtocolTCP),
		Port:       defaultPort,
		TargetPort: defaultPort,
	}
}

type inspectParams struct {
	sourceImage       string
	podName           string
	destinationImages []string
	stdout            io.Writer
	stderr            io.Writer
	eventsOutput      io.Writer
	client            *ClusterClient
	labels            *provision.LabelSet
	app               provision.App
}

func runInspectSidecar(ctx context.Context, params inspectParams) error {
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
	for i, c := range pod.Spec.Containers {
		if c.Name != inspectContainer {
			pod.Spec.Containers[i].ImagePullPolicy = apiv1.PullAlways
		}
	}

	ns, err := params.client.AppNamespace(params.app)
	if err != nil {
		return err
	}

	events, err := params.client.CoreV1().Events(ns).List(listOptsForPodEvent(params.podName))
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = params.client.CoreV1().Pods(ns).Create(&pod)
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(params.client, pod.Name, ns)

	closeFn, err := logPodEvents(params.client, events.ResourceVersion, params.podName, ns, params.eventsOutput)
	if err != nil {
		return err
	}
	defer closeFn()

	tctx, cancel := context.WithTimeout(ctx, kubeConf.PodRunningTimeout)
	err = waitForPodContainersRunning(tctx, params.client, &pod, ns)
	cancel()
	if err != nil {
		messages, msgErr := notReadyPodEventsForPod(params.client, params.podName, ns)
		if msgErr != nil {
			return err
		}
		var msgsStr []string
		for _, m := range messages {
			msgsStr = append(msgsStr, fmt.Sprintf(" ---> %s", m.message))
		}
		return errors.New(strings.Join(msgsStr, "\n"))
	}

	err = doAttach(ctx, params.client, bytes.NewBufferString("."), params.stdout, params.stderr, pod.Name, inspectContainer, false, nil, ns)
	if err != nil {
		return err
	}

	tctx, cancel = context.WithTimeout(ctx, kubeConf.PodReadyTimeout)
	defer cancel()
	return waitForPod(tctx, params.client, &pod, ns, false)
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
	serviceLinks := false
	return apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			EnableServiceLinks: &serviceLinks,
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
				newSleepyContainer(podName, sourceImage, uid, appEnvs(app, "", nil, true), mounts...),
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
					VolumeMounts: []apiv1.VolumeMount{
						{Name: "dockersock", MountPath: dockerSockPath},
					},
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
		VolumeMounts: []apiv1.VolumeMount{
			{Name: "dockersock", MountPath: dockerSockPath},
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		},
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

func deepCopyPorts(ports []apiv1.ServicePort) []apiv1.ServicePort {
	result := make([]apiv1.ServicePort, len(ports))
	for i := range ports {
		result[i] = *ports[i].DeepCopy()
	}
	return result
}
