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

	"github.com/docker/cli/cli/config/configfile"
	dockerclitypes "github.com/docker/cli/cli/config/types"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	routerTypes "github.com/tsuru/tsuru/types/router"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
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
	dockerConfigVolume      = "dockerconfig"
	dockerSockVolume        = "dockersock"
	dockerSockPath          = "/var/run/docker.sock"
	containerdRunVolume     = "containerd-run"
	containerdRunDir        = "/run/containerd"
	buildIntercontainerPath = "/tmp/intercontainer"
	buildIntercontainerDone = buildIntercontainerPath + "/done"
	defaultHttpPortName     = "http-default"
	defaultUdpPortName      = "udp-default"
	backendConfigCRDName    = "backendconfigs.cloud.google.com"
	backendConfigKey        = "cloud.google.com/backend-config"
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
	upgradeRoundTripper := spdy.NewRoundTripper(tlsConfig, true, false)
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
	quota             apiv1.ResourceRequirements
	mainContainer     string
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
	pod, err := newDeployAgentImageBuildPod(ctx, params.client, params.sourceImage, params.podName, deployAgentConfig{
		name:              params.mainContainer,
		image:             params.client.deploySidecarImage(),
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

func getImagePullSecrets(ctx context.Context, client *ClusterClient, namespace string, images ...string) ([]apiv1.LocalObjectReference, error) {
	reg := registryAuth("")
	useSecret := false
	for _, img := range images {
		imgDomain, _, _ := image.ParseImageParts(img)
		if imgDomain == reg.imgDomain {
			useSecret = true
			break
		}
	}
	if !useSecret {
		return nil, nil
	}
	secretName, err := ensureAuthSecret(ctx, client, namespace, reg)
	if err != nil {
		return nil, err
	}
	if secretName == "" {
		return nil, nil
	}
	return []apiv1.LocalObjectReference{
		{Name: secretName},
	}, nil
}

func ensureAuthSecret(ctx context.Context, client *ClusterClient, namespace string, reg registryAuthConfig) (string, error) {
	var cf configfile.ConfigFile
	dc := client.dockerConfigJSON()
	if dc != "" {
		if err := json.Unmarshal([]byte(dc), &cf); err != nil {
			return "", errors.Wrap(err, "could not decode custom Docker config from JSON")
		}
	}
	if reg.username == "" && reg.password == "" && dc == "" {
		return "", nil
	}
	if reg.username != "" || reg.password != "" {
		if cf.AuthConfigs == nil {
			cf.AuthConfigs = make(map[string]dockerclitypes.AuthConfig)
		}
		cf.AuthConfigs[reg.imgDomain] = dockerclitypes.AuthConfig{
			Username: reg.username,
			Password: reg.password,
			Auth:     base64.StdEncoding.EncodeToString([]byte(reg.username + ":" + reg.password)),
		}
	}
	serializedConf, err := json.Marshal(cf)
	if err != nil {
		return "", errors.Wrap(err, "could not encode Docker config to JSON")
	}
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker-config-tsuru",
			Namespace: namespace,
		},
		Type: apiv1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			apiv1.DockerConfigJsonKey: serializedConf,
		},
	}
	_, err = client.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	}
	if err != nil {
		err = errors.WithStack(err)
	}
	return secret.Name, err
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
		pod, err := newDeployAgentPod(ctx, params, deployAgentConfig{
			name:              params.mainContainer,
			image:             params.client.deploySidecarImage(),
			cmd:               fmt.Sprintf("mkdir -p $(dirname %[1]s) && cat >%[1]s && %[2]s", params.inputFile, strings.Join(params.cmds[2:], " ")),
			destinationImages: params.destinationImages,
			inputFile:         params.inputFile,
		})
		if err != nil {
			return err
		}
		applyAppMetadata(&pod, params.app)
		params.pod = &pod
	}
	ns, err := params.client.AppNamespace(ctx, params.app)
	if err != nil {
		return err
	}
	events, err := params.client.CoreV1().Events(ns).List(ctx, listOptsForResourceEvent("Pod", params.podName))
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = params.client.CoreV1().Pods(ns).Create(ctx, params.pod, metav1.CreateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	closeFn, err := logPodEvents(ctx, params.client, events.ResourceVersion, params.podName, ns, params.attachOutput)
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

func applyAppMetadata(pod *apiv1.Pod, app provision.App) {
	if app == nil {
		return
	}
	metadata := app.GetMetadata()
	for _, l := range metadata.Labels {
		pod.Labels[l.Name] = l.Value
	}
	for _, annotation := range metadata.Annotations {
		pod.Annotations[annotation.Name] = annotation.Value
	}
}

type registryAuthConfig struct {
	username  string
	password  string
	imgDomain string
	insecure  bool
}

func registryAuth(img string) registryAuthConfig {
	regDomain, _ := config.GetString("docker:registry")
	if img != "" {
		imgDomain, _, _ := image.ParseImageParts(img)
		if imgDomain != regDomain {
			return registryAuthConfig{}
		}
	}
	username, _ := config.GetString("docker:registry-auth:username")
	password, _ := config.GetString("docker:registry-auth:password")
	insecure, _ := config.GetBool("docker:registry-auth:insecure")
	return registryAuthConfig{
		username:  username,
		password:  password,
		imgDomain: regDomain,
		insecure:  insecure,
	}
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

func logPodEvents(ctx context.Context, client *ClusterClient, initialResourceVersion, podName, namespace string, output io.Writer) (func(), error) {
	watch, err := filteredResourceEvents(ctx, client, initialResourceVersion, "Pod", podName, namespace)
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

func ensureHealthCheckDefaults(hc *provTypes.TsuruYamlHealthcheck) error {
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
		return errors.New("healthcheck: only GET method is supported in kubernetes provisioner")
	}

	return nil
}

func probesFromHC(hc *provTypes.TsuruYamlHealthcheck, client *ClusterClient, port int) (hcResult, error) {
	var result hcResult
	if hc == nil || (hc.Path == "" && len(hc.Command) == 0) {
		return result, nil
	}
	if err := ensureHealthCheckDefaults(hc); err != nil {
		return result, err
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

func ensureNamespaceForApp(ctx context.Context, client *ClusterClient, app provision.App) error {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}
	return ensureNamespace(ctx, client, ns)
}

func ensurePoolNamespace(ctx context.Context, client *ClusterClient, pool string) error {
	return ensureNamespace(ctx, client, client.PoolNamespace(pool))
}

func ensureNamespace(ctx context.Context, client *ClusterClient, namespace string) error {
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
	_, err = client.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return errors.WithStack(err)
	}
	return nil
}

func ensureServiceAccount(ctx context.Context, client *ClusterClient, name string, labels *provision.LabelSet, namespace string, appMeta *appTypes.Metadata) error {
	var annotations map[string]string
	if appMeta != nil {
		saAnnotationsRaw, ok := appMeta.Annotation(AnnotationServiceAccountAnnotations)
		if ok {
			json.Unmarshal([]byte(saAnnotationsRaw), &annotations)
		} else {
			saAnnotationsRaw, ok = appMeta.Annotation(ResourceMetadataPrefix + "service-account")
			if ok {
				json.Unmarshal([]byte(saAnnotationsRaw), &annotations)
			}
		}
	}

	svcAccount := apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels.ToLabels(),
			Annotations: annotations,
		},
	}
	existingSA, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, svcAccount.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingSA = nil
	} else if err != nil {
		return errors.WithStack(err)
	}

	if existingSA == nil {
		_, err = client.CoreV1().ServiceAccounts(namespace).Create(ctx, &svcAccount, metav1.CreateOptions{})
	} else {
		svcAccount.ResourceVersion = existingSA.ResourceVersion
		svcAccount.Finalizers = existingSA.Finalizers
		svcAccount.Secrets = existingSA.Secrets
		_, err = client.CoreV1().ServiceAccounts(namespace).Update(ctx, &svcAccount, metav1.UpdateOptions{})
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ensureServiceAccountForApp(ctx context.Context, client *ClusterClient, a provision.App) error {
	labels := provision.ServiceAccountLabels(provision.ServiceAccountLabelsOpts{
		App:         a,
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	appMeta := a.GetMetadata()
	return ensureServiceAccount(ctx, client, serviceAccountNameForApp(a), labels, ns, &appMeta)
}

func getClusterNodeSelectorFlag(client *ClusterClient) (bool, error) {
	shouldDisable := false
	if val, ok := client.GetCluster().CustomData[disableDefaultNodeSelectorKey]; ok {
		var err error
		shouldDisable, err = strconv.ParseBool(val)
		if err != nil {
			return false, errors.WithMessage(err, fmt.Sprintf("error while parsing cluster custom data entry: %s", disableDefaultNodeSelectorKey))
		}
	}
	return shouldDisable, nil
}

func defineSelectorAndAffinity(ctx context.Context, a provision.App, client *ClusterClient) (map[string]string, *apiv1.Affinity, error) {
	singlePool, err := client.SinglePool()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster single pool value")
	}
	if singlePool {
		return nil, nil, nil
	}

	pool, err := pool.GetPoolByName(ctx, a.GetPool())
	if err != nil {
		return nil, nil, err
	}
	affinity, err := pool.GetAffinity()
	if err != nil {
		return nil, nil, err
	}
	if affinity != nil && affinity.NodeAffinity != nil {
		return nil, affinity, nil
	}

	shouldDisable, err := getClusterNodeSelectorFlag(client)
	if err != nil {
		return nil, nil, err
	}
	if shouldDisable {
		return nil, affinity, nil
	}

	return provision.NodeLabels(provision.NodeLabelsOpts{
		Pool:   a.GetPool(),
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector(), affinity, nil
}

func createAppDeployment(ctx context.Context, client *ClusterClient, depName string, oldDeployment *appsv1.Deployment, a provision.App, process string, version appTypes.AppVersion, replicas int, labels *provision.LabelSet, selector map[string]string, w io.Writer) (*appsv1.Deployment, *provision.LabelSet, error) {
	realReplicas := int32(replicas)
	extra := []string{}

	if client.unitRegisterCmdEnabled() {
		extra = []string{extraRegisterCmds(a)}
	}

	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(version)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	cmds, _, err := dockercommon.LeanContainerCmdsWithExtra(process, cmdData, a, extra)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	tenRevs := int32(10)
	webProcessName, err := version.WebProcess()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	yamlData, err := version.TsuruYamlData()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	processPorts, err := getProcessPortsForVersion(version, process)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	var hcData hcResult
	if process == webProcessName && len(processPorts) > 0 {
		//TODO: add support to multiple HCs
		hcData, err = probesFromHC(yamlData.Healthcheck, client, processPorts[0].TargetPort)
		if err != nil {
			return nil, nil, err
		}
	}

	sleepSec := client.preStopSleepSeconds(a.GetPool())
	terminationGracePeriod := int64(30 + sleepSec)

	var lifecycle apiv1.Lifecycle
	if sleepSec > 0 {
		lifecycle.PreStop = &apiv1.Handler{
			Exec: &apiv1.ExecAction{
				// Allow some time for endpoints controller and kube-proxy to
				// remove the endpoints for the pods before sending SIGTERM to
				// app. This should reduce the number of failed connections due
				// to pods stopping while their endpoints are still active.
				Command: []string{"sh", "-c", fmt.Sprintf("sleep %d || true", sleepSec)},
			},
		}
	}

	if yamlData.Hooks != nil && len(yamlData.Hooks.Restart.After) > 0 {
		hookCmds := []string{
			"sh", "-c",
			strings.Join(yamlData.Hooks.Restart.After, " && "),
		}
		lifecycle.PostStart = &apiv1.Handler{
			Exec: &apiv1.ExecAction{
				Command: hookCmds,
			},
		}
	}
	maxSurge := client.maxSurge(a.GetPool())
	maxUnavailable := client.maxUnavailable(a.GetPool())
	dnsConfig := dnsConfigNdots(client, a)
	nodeSelector, affinity, err := defineSelectorAndAffinity(ctx, a, client)
	if err != nil {
		return nil, nil, err
	}

	_, uid := dockercommon.UserForContainer()
	overCommit, err := client.OvercommitFactor(a.GetPool())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster overcommit factor")
	}
	cpuOverCommit, err := client.CPUOvercommitFactor(a.GetPool())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster cpu overcommit factor")
	}
	cpuBurst, err := client.CPUBurstFactor(a.GetPool())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster cpu burst factor")
	}
	memoryOverCommit, err := client.MemoryOvercommitFactor(a.GetPool())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster memory overcommit factor")
	}
	resourceRequirements, err := appResourceRequirements(a, client, requirementsFactors{
		overCommit:       overCommit,
		cpuOverCommit:    cpuOverCommit,
		cpuBurst:         cpuBurst,
		memoryOverCommit: memoryOverCommit,
	})
	if err != nil {
		return nil, nil, err
	}
	volumes, mounts, err := createVolumesForApp(ctx, client, a)
	if err != nil {
		return nil, nil, err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	deployImage := version.VersionInfo().DeployImage
	pullSecrets, err := getImagePullSecrets(ctx, client, ns, deployImage)
	if err != nil {
		return nil, nil, err
	}

	metadata := a.GetMetadata()
	for _, l := range metadata.Labels {
		labels.RawLabels[l.Name] = l.Value
	}

	annotations := map[string]string{}
	for _, annotation := range metadata.Annotations {
		annotations[annotation.Name] = annotation.Value
	}

	depLabels := labels.WithoutVersion().ToLabels()
	podLabels := labels.ToLabels()
	containerPorts := make([]apiv1.ContainerPort, len(processPorts))
	for i, port := range processPorts {
		portInt := port.TargetPort
		if portInt == 0 {
			portInt, _ = strconv.Atoi(provision.WebProcessDefaultPort())
		}
		containerPorts[i].ContainerPort = int32(portInt)
	}
	serviceLinks := false

	routers := a.GetRouters()
	conditionSet := set.Set{}
	for _, r := range routers {
		var planRouter routerTypes.PlanRouter
		_, planRouter, err = router.GetWithPlanRouter(ctx, r.Name)
		if err != nil {
			return nil, nil, err
		}
		for _, condition := range planRouter.ReadinessGates {
			conditionSet.Add(condition)
		}
	}
	var readinessGates []apiv1.PodReadinessGate

	if process == webProcessName {
		for condition := range conditionSet {
			readinessGates = append(readinessGates, apiv1.PodReadinessGate{
				ConditionType: apiv1.PodConditionType(condition),
			})
		}
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        depName,
			Namespace:   ns,
			Labels:      depLabels,
			Annotations: annotations,
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
					Annotations: annotations,
				},
				Spec: apiv1.PodSpec{
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					EnableServiceLinks:            &serviceLinks,
					ImagePullSecrets:              pullSecrets,
					ServiceAccountName:            serviceAccountNameForApp(a),
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: uid,
					},
					RestartPolicy:  apiv1.RestartPolicyAlways,
					NodeSelector:   nodeSelector,
					Affinity:       affinity,
					Volumes:        volumes,
					Subdomain:      headlessServiceName(a, process),
					ReadinessGates: readinessGates,
					DNSConfig:      dnsConfig,
					Containers: []apiv1.Container{
						{
							Name:           depName,
							Image:          deployImage,
							Command:        cmds,
							Env:            appEnvs(a, process, version, false),
							ReadinessProbe: hcData.readiness,
							LivenessProbe:  hcData.liveness,
							Resources:      resourceRequirements,
							VolumeMounts:   mounts,
							Ports:          containerPorts,
							Lifecycle:      &lifecycle,
						},
					},
				},
			},
		},
	}
	var newDep *appsv1.Deployment
	if oldDeployment == nil {
		newDep, err = client.AppsV1().Deployments(ns).Create(ctx, &deployment, metav1.CreateOptions{})
	} else {
		newDep, err = client.AppsV1().Deployments(ns).Update(ctx, &deployment, metav1.UpdateOptions{})
	}
	return newDep, labels, errors.WithStack(err)
}

func appEnvs(a provision.App, process string, version appTypes.AppVersion, isDeploy bool) []apiv1.EnvVar {
	appEnvs := EnvsForApp(a, process, version, isDeploy)
	envs := make([]apiv1.EnvVar, len(appEnvs))
	for i, envData := range appEnvs {
		envs[i] = apiv1.EnvVar{
			Name:  envData.Name,
			Value: strings.ReplaceAll(envData.Value, "$", "$$"),
		}
	}

	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name }) // just for testing reasons

	return envs
}

type serviceManager struct {
	client *ClusterClient
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) CleanupServices(ctx context.Context, a provision.App, deployedVersion int, preserveOldVersions bool) error {
	depGroups, err := deploymentsDataForApp(ctx, m.client, a)
	if err != nil {
		return err
	}

	type processVersionKey struct {
		process string
		version int
	}

	fmt.Fprint(m.writer, "\n---- Cleaning up resources ----\n")

	baseVersion, err := baseVersionForApp(ctx, m.client, a)
	if err != nil {
		return err
	}

	processInUse := map[string]struct{}{}
	versionInUse := map[processVersionKey]struct{}{}
	multiErrors := tsuruErrors.NewMultiError()
	for _, depsData := range depGroups.versioned {
		for _, depData := range depsData {
			toKeep := (depData.isBase && depData.version == baseVersion) ||
				(depData.replicas > 0 && (preserveOldVersions || depData.version == deployedVersion))
			if toKeep {
				processInUse[depData.process] = struct{}{}
				versionInUse[processVersionKey{process: depData.process, version: depData.version}] = struct{}{}
				continue
			}

			fmt.Fprintf(m.writer, " ---> Cleaning up deployment %s\n", depData.dep.Name)
			err = cleanupSingleDeployment(ctx, m.client, depData.dep)
			if err != nil {
				multiErrors.Add(err)
			}
		}
	}

	svcs, err := allServicesForApp(ctx, m.client, a)
	if err != nil {
		multiErrors.Add(err)
	}
	for _, svc := range svcs {
		labels := labelSetFromMeta(&svc.ObjectMeta)
		svcVersion := labels.AppVersion()
		process := labels.AppProcess()
		_, inUseProcess := processInUse[process]
		_, inUseVersion := versionInUse[processVersionKey{process: labels.AppProcess(), version: svcVersion}]

		toKeep := inUseVersion || (svcVersion == 0 && inUseProcess)

		if toKeep {
			continue
		}

		fmt.Fprintf(m.writer, " ---> Cleaning up service %s\n", svc.Name)
		err = m.client.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{
			PropagationPolicy: propagationPtr(metav1.DeletePropagationForeground),
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			multiErrors.Add(err)
		}
	}

	pdbs, err := allPDBsForApp(ctx, m.client, a)
	if err != nil {
		multiErrors.Add(err)
	}
	for _, pdb := range pdbs {
		labels := labelSetFromMeta(&pdb.ObjectMeta)
		process := labels.AppProcess()
		if _, toKeep := processInUse[process]; toKeep {
			continue
		}

		fmt.Fprintf(m.writer, " ---> Cleaning up PodDisruptionBudget %s\n", pdb.Name)
		err = m.client.PolicyV1beta1().PodDisruptionBudgets(pdb.Namespace).Delete(ctx, pdb.Name, metav1.DeleteOptions{})
		if err != nil {
			multiErrors.Add(err)
		}
	}

	return multiErrors.ToError()
}

func (m *serviceManager) RemoveService(ctx context.Context, a provision.App, process string, versionNumber int) error {
	multiErrors := tsuruErrors.NewMultiError()
	err := cleanupDeployment(ctx, m.client, a, process, versionNumber)
	if err != nil && !k8sErrors.IsNotFound(err) {
		multiErrors.Add(err)
	}
	err = cleanupServices(ctx, m.client, a, process, versionNumber)
	if err != nil {
		multiErrors.Add(err)
	}
	return multiErrors.ToError()
}

func (m *serviceManager) CurrentLabels(ctx context.Context, a provision.App, process string, versionNumber int) (*provision.LabelSet, *int32, error) {
	dep, err := deploymentForVersion(ctx, m.client, a, process, versionNumber)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	depLabels := labelOnlySetFromMetaPrefix(&dep.ObjectMeta, false)
	podLabels := labelOnlySetFromMetaPrefix(&dep.Spec.Template.ObjectMeta, false)
	return depLabels.Merge(podLabels), dep.Spec.Replicas, nil
}

const deadlineExeceededProgressCond = "ProgressDeadlineExceeded"

func createDeployTimeoutError(ctx context.Context, client *ClusterClient, ns string, selector map[string]string, w io.Writer, timeout time.Duration, label string) error {
	messages, err := notReadyPodEvents(ctx, client, ns, selector)
	if err != nil {
		return errors.Wrap(err, "Unknown error deploying application")
	}
	if len(messages) == 0 {
		// This should not be possible.
		return errors.Errorf("Unknown error deploying application, timeout after %v", timeout)
	}
	var msgsStr []string
	var pods []*apiv1.Pod

	crashedUnitsSet := make(map[string]struct{})
	for i, m := range messages {
		crashedUnitsSet[m.pod.Name] = struct{}{}
		msgsStr = append(msgsStr, fmt.Sprintf(" ---> %s", m.message))
		pods = append(pods, &messages[i].pod)
	}
	var crashedUnits []string
	for u := range crashedUnitsSet {
		crashedUnits = append(crashedUnits, u)
	}

	var crashedUnitsLogs []appTypes.Applog

	if client.LogsFromAPIServerEnabled() {
		crashedUnitsLogs, err = listLogsFromPods(ctx, client, ns, pods, appTypes.ListLogArgs{
			Limit: 10,
		})

		if err != nil {
			return errors.Wrap(err, "Could not get logs from crashed units")
		}
	}

	return provision.ErrUnitStartup{
		CrashedUnits:     crashedUnits,
		CrashedUnitsLogs: crashedUnitsLogs,
		Err:              errors.New(strings.Join(msgsStr, "\n")),
	}
}

func filteredResourceEvents(ctx context.Context, client *ClusterClient, evtResourceVersion, resourceType, resourceName, namespace string) (watch.Interface, error) {
	var err error
	client, err = NewClusterClient(client.Cluster)
	if err != nil {
		return nil, err
	}
	err = client.SetTimeout(time.Hour)
	if err != nil {
		return nil, err
	}
	opts := listOptsForResourceEvent(resourceType, resourceName)
	opts.Watch = true
	opts.ResourceVersion = evtResourceVersion
	evtWatch, err := client.CoreV1().Events(namespace).Watch(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return evtWatch, nil
}

func listOptsForResourceEvent(resourceType, resourceName string) metav1.ListOptions {
	selector := map[string]string{
		"involvedObject.kind": resourceType,
	}
	if resourceName != "" {
		selector["involvedObject.name"] = resourceName
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
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return revision, err
	}
	watchPods, err := filteredResourceEvents(ctx, client, evtResourceVersion, "Pod", "", ns)
	if err != nil {
		return revision, err
	}
	watchDep, err := filteredResourceEvents(ctx, client, evtResourceVersion, "Deployment", dep.Name, ns)
	if err != nil {
		return revision, err
	}
	watchReplicaSet, err := filteredResourceEvents(ctx, client, evtResourceVersion, "ReplicaSet", "", ns)
	if err != nil {
		return revision, err
	}
	watchPodCh := watchPods.ResultChan()
	watchDepCh := watchDep.ResultChan()
	watchRepCh := watchReplicaSet.ResultChan()
	defer func() {
		watchPods.Stop()
		if watchPodCh != nil {
			// Drain watch channel to avoid goroutine leaks.
			<-watchPodCh
		}
		watchDep.Stop()
		if watchDepCh != nil {
			// Drain watch channel to avoid goroutine leaks.
			<-watchDepCh
		}
		watchReplicaSet.Stop()
		if watchRepCh != nil {
			// Drain watch channel to avoid goroutine leaks.
			<-watchRepCh
		}
	}()

	fmt.Fprintf(w, "\n---- Updating units [%s] [version %d] ----\n", processName, version.Version())
	kubeConf := getKubeConfig()
	timer := time.NewTimer(kubeConf.DeploymentProgressTimeout)
	for dep.Status.ObservedGeneration < dep.Generation {
		dep, err = client.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

	oldUpdatedReplicas := int32(-1)
	oldReadyUnits := int32(-1)
	oldPendingTermination := int32(-1)
	tsuruYamlData, err := version.TsuruYamlData()
	if err != nil {
		return revision, errors.WithStack(err)
	}
	maxWaitTimeDuration := dockercommon.DeployHealthcheckTimeout(tsuruYamlData)
	var healthcheckTimeout <-chan time.Time
	t0 := time.Now()
	largestReady := int32(0)
	for {
		var specReplicas int32
		if dep.Spec.Replicas != nil {
			specReplicas = *dep.Spec.Replicas
		}
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
			allInit, err = allNewPodsRunning(ctx, client, a, processName, dep, version)
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
		case msg, isOpen := <-watchPodCh:
			if !isOpen {
				watchPodCh = nil
				break
			}
			if isDeploymentEvent(msg, dep) {
				fmt.Fprintf(w, "  ---> %s\n", formatEvtMessage(msg, false))
			}
		case msg, isOpen := <-watchDepCh:
			if !isOpen {
				watchDepCh = nil
				break
			}
			fmt.Fprintf(w, "  ---> %s\n", formatEvtMessage(msg, false))

		case msg, isOpen := <-watchRepCh:
			if !isOpen {
				watchRepCh = nil
				break
			}
			if isDeploymentEvent(msg, dep) {
				fmt.Fprintf(w, "  ---> %s\n", formatEvtMessage(msg, false))
			}
		case <-healthcheckTimeout:
			return revision, createDeployTimeoutError(ctx, client, ns, dep.Spec.Selector.MatchLabels, w, time.Since(t0), "healthcheck")
		case <-timer.C:
			return revision, createDeployTimeoutError(ctx, client, ns, dep.Spec.Selector.MatchLabels, w, time.Since(t0), "full rollout")
		case <-ctx.Done():
			err = ctx.Err()
			if err == context.Canceled {
				err = errors.Wrap(err, "canceled by user action")
			}
			return revision, err
		}
		dep, err = client.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
		if err != nil {
			return revision, err
		}
	}
	fmt.Fprintln(w, " ---> Done updating units")
	return revision, nil
}

func (m *serviceManager) DeployService(ctx context.Context, opts servicecommon.DeployServiceOpts) error {
	if m.writer == nil {
		m.writer = ioutil.Discard
	}

	err := ensureNodeContainers(opts.App)
	if err != nil {
		return err
	}
	err = ensureNamespaceForApp(ctx, m.client, opts.App)
	if err != nil {
		return err
	}
	err = ensureServiceAccountForApp(ctx, m.client, opts.App)
	if err != nil {
		return err
	}
	ns, err := m.client.AppNamespace(ctx, opts.App)
	if err != nil {
		return err
	}

	provision.ExtendServiceLabels(opts.Labels, provision.ServiceLabelExtendedOpts{
		Provisioner: provisionerName,
		Prefix:      tsuruLabelPrefix,
	})

	depArgs, err := m.baseDeploymentArgs(ctx, opts.App, opts.ProcessName, opts.Labels, opts.Version, opts.PreserveVersions)
	if err != nil {
		return err
	}

	if depArgs.baseDep != nil && depArgs.baseDep.isLegacy && depArgs.name != depArgs.baseDep.dep.Name {
		fmt.Fprint(m.writer, "\n---- Updating legacy deployment ----\n")
		err = toggleRoutableDeployment(ctx, m.client, depArgs.baseDep.version, depArgs.baseDep.dep, true)
		if err != nil {
			return errors.Wrap(err, "unable to update legacy deployment")
		}
	}

	oldDep, err := m.client.AppsV1().Deployments(ns).Get(ctx, depArgs.name, metav1.GetOptions{})
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
	events, err := m.client.CoreV1().Events(ns).List(ctx, listOptsForResourceEvent("Pod", ""))
	if err != nil {
		return errors.WithStack(err)
	}

	if opts.OverrideVersions {
		var deps *appsv1.DeploymentList
		processSelector := fmt.Sprintf("tsuru.io/app-name=%s, tsuru.io/app-process=%s, tsuru.io/is-routable=true", opts.App.GetName(), opts.ProcessName)
		deps, err = m.client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
			LabelSelector: processSelector,
		})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
		totalReplicas := 0
		if deps != nil {
			for _, dep := range deps.Items {
				if dep.Spec.Replicas != nil {
					totalReplicas += int(*dep.Spec.Replicas)
				}
			}
			if totalReplicas > 0 {
				opts.Replicas = totalReplicas
			}
		}
	}

	newDep, labels, err := createAppDeployment(ctx, m.client, depArgs.name, oldDep, opts.App, opts.ProcessName, opts.Version, opts.Replicas, opts.Labels, depArgs.selector, m.writer)
	if err != nil {
		return err
	}
	newRevision, err := monitorDeployment(ctx, m.client, newDep, opts.App, opts.ProcessName, m.writer, events.ResourceVersion, opts.Version)
	if err != nil {
		// We should only rollback if the updated deployment is a new revision.
		var rollbackErr error
		if oldDep != nil && (newRevision == "" || oldRevision == newRevision) {
			oldDep.Generation = 0
			oldDep.ResourceVersion = ""
			fmt.Fprintf(m.writer, "\n**** UPDATING BACK AFTER FAILURE ****\n")
			_, rollbackErr = m.client.AppsV1().Deployments(ns).Update(ctx, oldDep, metav1.UpdateOptions{})
		} else if oldDep == nil {
			// We have just created the deployment, so we need to remove it
			fmt.Fprintf(m.writer, "\n**** DELETING CREATED DEPLOYMENT AFTER FAILURE ****\n")
			rollbackErr = m.client.AppsV1().Deployments(ns).Delete(ctx, newDep.Name, metav1.DeleteOptions{})
		} else {
			fmt.Fprintf(m.writer, "\n**** ROLLING BACK AFTER FAILURE ****\n")

			// This code was copied from kubernetes codebase, in the next update of version of kubectl
			// we need to move to import this library:
			// https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/kubectl/pkg/polymorphichelpers/rollback.go#L48
			rollbacker := &DeploymentRollbacker{c: m.client}
			rollbackErr = rollbacker.Rollback(ctx, m.writer, newDep)
		}
		if rollbackErr != nil {
			fmt.Fprintf(m.writer, "\n**** ERROR DURING ROLLBACK ****\n ---> %s <---\n", rollbackErr)
		}
		if _, ok := err.(provision.ErrUnitStartup); ok {
			return err
		}
		return provision.ErrUnitStartup{Err: err}
	}

	backendCfgexists, err := ensureBackendConfig(ctx, backendConfigArgs{
		client:  m.client,
		app:     opts.App,
		process: opts.ProcessName,
		writer:  m.writer,
		version: opts.Version,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(m.writer, "\n---- Ensuring services [%s] ----\n", opts.ProcessName)
	err = m.ensureServices(ctx, opts.App, opts.ProcessName, labels, opts.Version, backendCfgexists, opts.PreserveVersions)
	if err != nil {
		return err
	}

	err = ensureAutoScale(ctx, m.client, opts.App, opts.ProcessName)
	if err != nil {
		return errors.Wrap(err, "unable to ensure auto scale is configured")
	}

	err = ensurePDB(ctx, m.client, opts.App, opts.ProcessName)
	if err != nil {
		return errors.Wrap(err, "unable to ensure pod disruption budget")
	}

	return nil
}

type baseDepArgs struct {
	name     string
	selector map[string]string
	isLegacy bool
	baseDep  *deploymentInfo
}

func (m *serviceManager) baseDeploymentArgs(ctx context.Context, a provision.App, process string, labels *provision.LabelSet, version appTypes.AppVersion, preserveVersions bool) (baseDepArgs, error) {
	var result baseDepArgs
	if !preserveVersions {
		labels.SetIsRoutable()
		result.name = deploymentNameForAppBase(a, process)
		result.selector = labels.ToBaseSelector()
		return result, nil
	}

	depData, err := deploymentsDataForProcess(ctx, m.client, a, process)
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

	baseVersion, err := baseVersionForApp(ctx, m.client, a)
	if err != nil {
		return result, err
	}

	if depData.base.dep != nil || (baseVersion != 0 && baseVersion != version.Version()) {
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
	name        string
	labels      map[string]string
	annotations map[string]string
	selector    map[string]string
	ports       []apiv1.ServicePort
}

func syncAnnotationMap(toAdd map[string]string, metadata map[string]string) {
	for key, value := range toAdd {
		metadata[key] = value
	}
}

func syncServiceAnnotations(app provision.App, svcData *svcCreateData) {
	metadata := app.GetMetadata()
	annotationsToAdd := make(map[string]string)
	annotationsRaw, ok := metadata.Annotation(ResourceMetadataPrefix + "service")
	if ok {
		json.Unmarshal([]byte(annotationsRaw), &annotationsToAdd)
		if svcData.annotations == nil {
			svcData.annotations = map[string]string{}
		}
		syncAnnotationMap(annotationsToAdd, svcData.annotations)
	}
}

func (m *serviceManager) ensureServices(ctx context.Context, a provision.App, process string, labels *provision.LabelSet, currentVersion appTypes.AppVersion, backendCRD, preserveOldVersions bool) error {
	ns, err := m.client.AppNamespace(ctx, a)
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

	routableLabels := labels.WithoutVersion().WithoutIsolated()
	routableLabels.SetIsRoutable()

	versionLabels := labels.WithoutRoutable()

	depData, err := deploymentsDataForProcess(ctx, m.client, a, process)
	if err != nil {
		return err
	}

	versions, err := servicemanager.AppVersion.AppVersions(ctx, a)
	if err != nil {
		return err
	}
	versionInfoMap := map[int]appTypes.AppVersionInfo{}
	for _, v := range versions.Versions {
		versionInfoMap[v.Version] = v
	}

	var baseSvcPorts []apiv1.ServicePort
	var svcsToCreate []svcCreateData

	baseVersion, err := baseVersionForApp(ctx, m.client, a)
	if err != nil {
		return err
	}

	createVersionedSvcs, err := m.client.EnableVersionedServices()
	if err != nil {
		return err
	}
	for versionNumber := range depData.versioned {
		if versionNumber != baseVersion && preserveOldVersions {
			createVersionedSvcs = true
			break
		}
	}

	for versionNumber, depInfo := range depData.versioned {
		if len(depInfo) == 0 {
			continue
		}

		vInfo, ok := versionInfoMap[versionNumber]
		if !ok {
			err = errors.Errorf("unable to ensure service %q, no version data found", serviceNameForApp(a, process, versionNumber))
			if currentVersion.Version() == versionNumber {
				return err
			} else {
				log.Error(err)
				continue
			}
		}
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.AppVersionFromInfo(ctx, a, vInfo)
		if err != nil {
			return err
		}
		var svcPorts []apiv1.ServicePort
		svcPorts, err = loadServicePorts(version, process)
		if err != nil {
			return err
		}

		if len(svcPorts) == 0 {
			err = cleanupServices(ctx, m.client, a, process, version.Version())
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

		if createVersionedSvcs {
			svcsToCreate = append(svcsToCreate, svcCreateData{
				name:     serviceNameForApp(a, process, versionNumber),
				labels:   labels.ToLabels(),
				selector: labels.ToVersionSelector(),
				ports:    svcPorts,
			})
		}

		if depInfo[0].isRoutable {
			baseSvcPorts = deepCopyPorts(svcPorts)
		}
	}

	if baseSvcPorts != nil {
		var annotations map[string]string
		annotations, err = m.client.ServiceAnnotations(baseServicesAnnotations)
		if err != nil {
			return errors.WithMessage(err, "could not to parse base services annotations")
		}
		if backendCRD {
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[backendConfigKey] = fmt.Sprintf("{\"default\":\"%s\"}", backendConfigNameForApp(a, process))
		}

		svcsToCreate = append(svcsToCreate, svcCreateData{
			name:        serviceNameForAppBase(a, process),
			labels:      routableLabels.ToLabels(),
			annotations: annotations,
			selector:    routableLabels.ToRoutableSelector(),
			ports:       baseSvcPorts,
		})
	}

	if len(svcsToCreate) == 0 {
		return nil
	}

	headlessPorts := deepCopyPorts(baseSvcPorts)

	addAllServicesAnnotations, err := m.client.ServiceAnnotations(allServicesAnnotations)
	if err != nil {
		return errors.WithMessage(err, "could not to parse all services annotations")
	}
	for _, svcData := range svcsToCreate {
		if addAllServicesAnnotations != nil {
			if svcData.annotations == nil {
				svcData.annotations = addAllServicesAnnotations
			}
			for k, v := range addAllServicesAnnotations {
				svcData.annotations[k] = v
			}
		}

		syncServiceAnnotations(a, &svcData)

		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcData.name,
				Namespace:   ns,
				Labels:      svcData.labels,
				Annotations: svcData.annotations,
			},
			Spec: apiv1.ServiceSpec{
				Selector:              svcData.selector,
				Ports:                 svcData.ports,
				Type:                  apiv1.ServiceTypeNodePort,
				ExternalTrafficPolicy: policy,
			},
		}
		var isNew bool
		svc, isNew, err = mergeServices(ctx, m.client, svc)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.writer, " ---> Service %s\n", svc.Name)
		if isNew {
			_, err = m.client.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
		} else {
			_, err = m.client.CoreV1().Services(svc.Namespace).Update(ctx, svc, metav1.UpdateOptions{})
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if baseVersion == currentVersion.Version() {
		err = m.createHeadlessService(ctx, headlessPorts, ns, a, process, routableLabels.WithoutVersion())
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *serviceManager) createHeadlessService(ctx context.Context, svcPorts []apiv1.ServicePort, ns string, a provision.App, process string, labels *provision.LabelSet) error {
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
			Name:      svcName,
			Namespace: ns,
			Labels:    expandedLabelsHeadless,
		},
		Spec: apiv1.ServiceSpec{
			Selector:  labels.ToRoutableSelector(),
			Ports:     headlessPorts,
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	}
	headlessSvc, isNew, err := mergeServices(ctx, m.client, headlessSvc)
	if err != nil {
		return err
	}
	if isNew {
		_, err = m.client.CoreV1().Services(ns).Create(ctx, headlessSvc, metav1.CreateOptions{})
	} else {
		_, err = m.client.CoreV1().Services(ns).Update(ctx, headlessSvc, metav1.UpdateOptions{})
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

func mergeServices(ctx context.Context, client *ClusterClient, svc *apiv1.Service) (*apiv1.Service, bool, error) {
	existing, err := client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
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
	// params.client, params.sourceImage, params.app, params.podName
	pod, err := newDeployAgentPod(ctx, createPodParams{
		client:      params.client,
		sourceImage: params.sourceImage,
		app:         params.app,
		podName:     params.podName,
	}, deployAgentConfig{
		name:              inspectContainer,
		image:             params.client.deployInspectImage(),
		cmd:               "cat >/dev/null && /bin/deploy-agent",
		destinationImages: params.destinationImages,
		sourceImage:       params.sourceImage,
	})
	applyAppMetadata(&pod, params.app)
	if err != nil {
		return err
	}
	for i, c := range pod.Spec.Containers {
		if c.Name != inspectContainer {
			pod.Spec.Containers[i].ImagePullPolicy = apiv1.PullAlways
		}
	}

	ns, err := params.client.AppNamespace(ctx, params.app)
	if err != nil {
		return err
	}

	events, err := params.client.CoreV1().Events(ns).List(ctx, listOptsForResourceEvent("Pod", params.podName))
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = params.client.CoreV1().Pods(ns).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanupPod(tsuruNet.WithoutCancel(ctx), params.client, pod.Name, ns)

	closeFn, err := logPodEvents(ctx, params.client, events.ResourceVersion, params.podName, ns, params.eventsOutput)
	if err != nil {
		return err
	}
	defer closeFn()

	tctx, cancel := context.WithTimeout(ctx, kubeConf.PodRunningTimeout)
	err = waitForPodContainersRunning(tctx, params.client, &pod, ns)
	cancel()
	if err != nil {
		messages, msgErr := notReadyPodEventsForPod(ctx, params.client, params.podName, ns)
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
	registryAuth      registryAuthConfig
	runAsUser         string
	dockerfileBuild   bool
}

func newDeployAgentPod(ctx context.Context, params createPodParams, conf deployAgentConfig) (apiv1.Pod, error) {
	dnsConfig := dnsConfigNdots(params.client, params.app)
	if len(conf.destinationImages) == 0 {
		return apiv1.Pod{}, errors.Errorf("no destination images provided")
	}
	err := ensureNamespaceForApp(ctx, params.client, params.app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	err = ensureServiceAccountForApp(ctx, params.client, params.app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: params.app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsBuild:     true,
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return apiv1.Pod{}, err
	}
	volumes, mounts, err := createVolumesForApp(ctx, params.client, params.app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	annotations := provision.LabelSet{Prefix: tsuruLabelPrefix}
	annotations.SetBuildImage(conf.destinationImages[0])

	nodeSelector, affinity, err := defineSelectorAndAffinity(ctx, params.app, params.client)
	if err != nil {
		return apiv1.Pod{}, err
	}
	_, uid := dockercommon.UserForContainer()
	ns, err := params.client.AppNamespace(ctx, params.app)
	if err != nil {
		return apiv1.Pod{}, err
	}
	pullSecrets, err := getImagePullSecrets(ctx, params.client, ns, params.sourceImage, conf.image)
	if err != nil {
		return apiv1.Pod{}, err
	}
	if uid != nil && conf.runAsUser == "" {
		conf.runAsUser = strconv.FormatInt(*uid, 10)
	}
	serviceLinks := false

	quota, err := resourceRequirementsForBuildPod(ctx, params.app, params.client)
	if err != nil {
		return apiv1.Pod{}, err
	}
	var deployAgentPlan apiv1.ResourceRequirements
	if buildPlan, ok := quota[buildPlanKey]; ok {
		params.quota = buildPlan
		deployAgentPlan = buildPlan
	}
	if buildPlanSidecar, ok := quota[buildPlanSideCarKey]; ok {
		deployAgentPlan = buildPlanSidecar
	}
	return apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        params.podName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			EnableServiceLinks: &serviceLinks,
			ImagePullSecrets:   pullSecrets,
			ServiceAccountName: params.client.buildServiceAccount(params.app),
			NodeSelector:       nodeSelector,
			Affinity:           affinity,
			DNSConfig:          dnsConfig,
			Volumes: append(deployAgentEngineVolumes(pullSecrets), append([]apiv1.Volume{
				{
					Name: "intercontainer",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			}, volumes...)...),
			RestartPolicy: apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				newSleepyContainer(params, uid, appEnvs(params.app, "", nil, true), mounts...),
				newDeployAgentContainer(conf, pullSecrets, deployAgentPlan),
			},
		},
	}, nil
}

func dnsConfigNdots(client *ClusterClient, app provision.App) *apiv1.PodDNSConfig {
	dnsConfigNdots := client.dnsConfigNdots(app.GetPool())
	var dnsConfig *apiv1.PodDNSConfig
	if dnsConfigNdots.IntVal > 0 {
		ndots := dnsConfigNdots.String()
		dnsConfig = &apiv1.PodDNSConfig{
			Options: []apiv1.PodDNSConfigOption{{
				Name:  "ndots",
				Value: &ndots,
			}},
		}
	}
	return dnsConfig
}

func newDeployAgentImageBuildPod(ctx context.Context, client *ClusterClient, sourceImage string, podName string, conf deployAgentConfig) (apiv1.Pod, error) {
	if len(conf.destinationImages) == 0 {
		return apiv1.Pod{}, errors.Errorf("no destination images provided")
	}
	err := ensureNamespaceForApp(ctx, client, nil)
	if err != nil {
		return apiv1.Pod{}, err
	}
	labels := provision.ImageBuildLabels(provision.ImageBuildLabelsOpts{
		IsBuild:     true,
		Prefix:      tsuruLabelPrefix,
		Provisioner: provisionerName,
	})
	annotations := provision.LabelSet{
		Prefix: tsuruLabelPrefix,
		RawLabels: map[string]string{
			fmt.Sprintf("container.apparmor.security.beta.kubernetes.io/%s", conf.name): "unconfined",
			fmt.Sprintf("container.seccomp.security.alpha.kubernetes.io/%s", conf.name): "unconfined",
		},
	}
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
	singlePool, err := client.SinglePool()
	if err != nil {
		return apiv1.Pod{}, errors.WithMessage(err, "misconfigured cluster single pool value")
	}
	shouldDisable, err := getClusterNodeSelectorFlag(client)
	if err != nil {
		return apiv1.Pod{}, err
	}
	if shouldDisable || singlePool {
		affinity = &apiv1.Affinity{}
	}
	_, uid := dockercommon.UserForContainer()
	ns := client.Namespace()
	pullSecrets, err := getImagePullSecrets(ctx, client, ns, sourceImage, conf.image, conf.destinationImages[0])
	if err != nil {
		return apiv1.Pod{}, err
	}
	if uid != nil && conf.runAsUser == "" {
		conf.runAsUser = strconv.FormatInt(*uid, 10)
	}
	conf.registryAuth = registryAuth(conf.destinationImages[0])
	return apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   ns,
			Labels:      labels.ToLabels(),
			Annotations: annotations.ToLabels(),
		},
		Spec: apiv1.PodSpec{
			Affinity:           affinity,
			ImagePullSecrets:   pullSecrets,
			Volumes:            deployAgentEngineVolumes(pullSecrets),
			RestartPolicy:      apiv1.RestartPolicyNever,
			ServiceAccountName: client.buildServiceAccount(nil),
			Containers: []apiv1.Container{
				{
					Name:         conf.name,
					Image:        conf.image,
					VolumeMounts: deployAgentEngineMounts(pullSecrets),
					Stdin:        true,
					StdinOnce:    true,
					Env:          conf.asEnvs(),
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
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: c.registryAuth.username},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: c.registryAuth.password},
		{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: c.registryAuth.imgDomain},
		{Name: "DEPLOYAGENT_INPUT_FILE", Value: c.inputFile},
		{Name: "DEPLOYAGENT_RUN_AS_USER", Value: c.runAsUser},
		{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: strconv.FormatBool(c.dockerfileBuild)},
		{Name: "DEPLOYAGENT_INSECURE_REGISTRY", Value: strconv.FormatBool(c.registryAuth.insecure)},
		{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
		{Name: "BUILDCTL_CONNECT_RETRIES_MAX", Value: "50"},
	}
}

func newDeployAgentContainer(conf deployAgentConfig, pullSecrets []apiv1.LocalObjectReference, quota apiv1.ResourceRequirements) apiv1.Container {
	conf.registryAuth = registryAuth(conf.destinationImages[0])
	privileged := true
	return apiv1.Container{
		Name:  conf.name,
		Image: conf.image,
		VolumeMounts: append(deployAgentEngineMounts(pullSecrets),
			apiv1.VolumeMount{Name: "intercontainer", MountPath: buildIntercontainerPath},
		),
		Stdin:     true,
		StdinOnce: true,
		Env:       conf.asEnvs(),
		Resources: quota,
		Command: []string{
			"sh", "-ec",
			fmt.Sprintf(`
				end() { touch %[1]s; }
				trap end EXIT
				%[2]s
			`, buildIntercontainerDone, conf.cmd),
		},
		SecurityContext: &apiv1.SecurityContext{
			// NOTE(nettoclaudio): deploy-agent container should be privileged to run buildctl on GKE.
			// See more: https://github.com/moby/buildkit/issues/2441
			Privileged: &privileged,
		},
	}
}

func newSleepyContainer(params createPodParams, uid *int64, envs []apiv1.EnvVar, mounts ...apiv1.VolumeMount) apiv1.Container {
	return apiv1.Container{
		Name:  params.podName,
		Image: params.sourceImage,
		Env:   envs,
		SecurityContext: &apiv1.SecurityContext{
			RunAsUser: uid,
		},
		Resources: params.quota,
		Command:   []string{"/bin/sh", "-ec", fmt.Sprintf("while [ ! -f %s ]; do sleep 5; done", buildIntercontainerDone)},
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

func deployAgentEngineVolumes(pullSecrets []apiv1.LocalObjectReference) []apiv1.Volume {
	vols := []apiv1.Volume{
		{
			Name: dockerSockVolume,
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: dockerSockPath,
				},
			},
		},
		{
			Name: containerdRunVolume,
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: containerdRunDir,
				},
			},
		},
	}
	if len(pullSecrets) == 0 {
		return vols
	}
	vols = append(vols, apiv1.Volume{
		Name: dockerConfigVolume,
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: pullSecrets[0].Name,
				Items: []apiv1.KeyToPath{
					{
						Key:  apiv1.DockerConfigJsonKey,
						Path: "config.json",
					},
				},
			},
		},
	})
	return vols
}

func deployAgentEngineMounts(pullSecrets []apiv1.LocalObjectReference) []apiv1.VolumeMount {
	mounts := []apiv1.VolumeMount{
		{Name: dockerSockVolume, MountPath: dockerSockPath},
		{Name: containerdRunVolume, MountPath: containerdRunDir},
	}
	if len(pullSecrets) == 0 {
		return mounts
	}
	mounts = append(mounts, apiv1.VolumeMount{
		Name:      dockerConfigVolume,
		MountPath: "/home/user/.docker",
	})
	return mounts
}
