// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/cli/cli/config/configfile"
	dockerclitypes "github.com/docker/cli/cli/config/types"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	"github.com/tsuru/tsuru/streamfmt"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	routerTypes "github.com/tsuru/tsuru/types/router"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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
	appSecretPrefix         = "tsuru-app-"
	secretHashAnnotationKey = "tsuru.io/secret-sha256"
)

func keepAliveSpdyExecutor(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}

	proxy := http.ProxyFromEnvironment
	if config.Proxy != nil {
		proxy = config.Proxy
	}

	upgradeRoundTripper, err := spdy.NewRoundTripperWithConfig(spdy.RoundTripperConfig{
		TLS:        tlsConfig,
		Proxier:    proxy,
		PingPeriod: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

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
		errCh <- doUnsafeAttach(ctx, client, stdin, stdout, stderr, podName, container, tty, size, namespace)
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

func doUnsafeAttach(ctx context.Context, client *ClusterClient, stdin io.Reader, stdout, stderr io.Writer, podName, container string, tty bool, size *remotecommand.TerminalSize, namespace string) error {
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

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
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
			streamfmt.FprintlnActionf(output, "%s", formatEvtMessage(msg, true))
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
	startup   *apiv1.Probe
}

func probesFromCheckConfigs(healthcheck *provTypes.TsuruYamlHealthcheck, startupcheck *provTypes.TsuruYamlStartupcheck, port int) (hcResult, error) {
	var result hcResult
	if healthcheck != nil && !healthcheck.IsEmpty() {
		hcProbe, err := assembleHealthProbe(healthcheck, port)
		if err != nil {
			return result, err
		}
		result.readiness = hcProbe
		if healthcheck.ForceRestart {
			result.liveness = hcProbe
		}
	}
	if startupcheck != nil && !startupcheck.IsEmpty() {
		scProbe, err := assembleStartupProbe(startupcheck, port)
		if err != nil {
			return result, err
		}
		result.startup = scProbe
	}
	return result, nil
}

func assembleHealthProbe(y *provTypes.TsuruYamlHealthcheck, port int) (*apiv1.Probe, error) {
	if err := y.EnsureDefaults(); err != nil {
		return nil, err
	}
	headers := []apiv1.HTTPHeader{}
	for header, value := range func() map[string]string {
		result := make(map[string]string)
		maps.Copy(result, y.Headers)
		return result
	}() {
		headers = append(headers, apiv1.HTTPHeader{Name: header, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	formatedScheme := strings.ToUpper(y.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: int32(y.AllowedFailures),
		PeriodSeconds:    int32(y.IntervalSeconds),
		TimeoutSeconds:   int32(y.TimeoutSeconds),
		ProbeHandler:     apiv1.ProbeHandler{},
	}
	if y.Path != "" {
		path := y.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		probe.ProbeHandler.HTTPGet = &apiv1.HTTPGetAction{
			Path:        path,
			Port:        intstr.FromInt(port),
			Scheme:      apiv1.URIScheme(formatedScheme),
			HTTPHeaders: headers,
		}
	} else {
		probe.ProbeHandler.Exec = &apiv1.ExecAction{
			Command: y.Command,
		}
	}
	return probe, nil
}

func assembleStartupProbe(y *provTypes.TsuruYamlStartupcheck, port int) (*apiv1.Probe, error) {
	if err := y.EnsureDefaults(); err != nil {
		return nil, err
	}
	headers := []apiv1.HTTPHeader{}
	for header, value := range func() map[string]string {
		result := make(map[string]string)
		maps.Copy(result, y.Headers)
		return result
	}() {
		headers = append(headers, apiv1.HTTPHeader{Name: header, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	formatedScheme := strings.ToUpper(y.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: int32(y.AllowedFailures),
		PeriodSeconds:    int32(y.IntervalSeconds),
		TimeoutSeconds:   int32(y.TimeoutSeconds),
		ProbeHandler:     apiv1.ProbeHandler{},
	}
	if y.Path != "" {
		path := y.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		probe.ProbeHandler.HTTPGet = &apiv1.HTTPGetAction{
			Path:        path,
			Port:        intstr.FromInt(port),
			Scheme:      apiv1.URIScheme(formatedScheme),
			HTTPHeaders: headers,
		}
	} else {
		probe.ProbeHandler.Exec = &apiv1.ExecAction{
			Command: y.Command,
		}
	}
	return probe, nil
}

func ensureNamespaceForApp(ctx context.Context, client *ClusterClient, app *appTypes.App) error {
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}
	return ensureNamespace(ctx, client, ns)
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

func ensureServiceAccount(ctx context.Context, client *ClusterClient, name string, labels *provision.LabelSet, namespace string, metadata *appTypes.Metadata) error {
	var annotations map[string]string
	if metadata != nil {
		if saAppAnnotationsRaw, ok := metadata.Annotation(AnnotationServiceAccountAppAnnotations); ok {
			json.Unmarshal([]byte(saAppAnnotationsRaw), &annotations)
		} else if saJobAnnotationsRaw, ok := metadata.Annotation(AnnotationServiceAccountJobAnnotations); ok {
			json.Unmarshal([]byte(saJobAnnotationsRaw), &annotations)
		} else {
			saAnnotationsRaw, ok := metadata.Annotation(ResourceMetadataPrefix + "service-account")
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

func ensureServiceAccountForApp(ctx context.Context, client *ClusterClient, a *appTypes.App) error {
	labels := provision.ServiceAccountLabels(provision.ServiceAccountLabelsOpts{
		App:    a,
		Prefix: tsuruLabelPrefix,
	})
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	appMeta := provision.GetAppMetadata(a, "")
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

func defineSelectorAndAffinity(ctx context.Context, a *appTypes.App, client *ClusterClient) (map[string]string, *apiv1.Affinity, error) {
	singlePool, err := client.SinglePool()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "misconfigured cluster single pool value")
	}
	if singlePool {
		return nil, nil, nil
	}

	pool, err := pool.GetPoolByName(ctx, a.Pool)
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
		Pool:   a.Pool,
		Prefix: tsuruLabelPrefix,
	}).ToNodeByPoolSelector(), affinity, nil
}

type ensureAppSecretOptions struct {
	writer     io.Writer
	client     *ClusterClient
	secretName string
	dep        *appsv1.Deployment
	app        *appTypes.App
	process    string
	version    appTypes.AppVersion
}

func ensureAppSecret(ctx context.Context, opts *ensureAppSecretOptions) (*apiv1.Secret, error) {
	ns, err := opts.client.AppNamespace(ctx, opts.app)
	if err != nil {
		return nil, err
	}
	oldSecret, err := opts.client.CoreV1().Secrets(ns).Get(ctx, opts.secretName, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return nil, errors.WithStack(err)
		}
		oldSecret = nil
	}
	envs := appSecretEnvs(opts.app, opts.process, opts.version)

	labels := provision.SecretLabels(provision.SecretLabelsOpts{
		App:    opts.app,
		Prefix: tsuruLabelPrefix,
	}).ToLabels()

	secret := apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.secretName,
			Namespace: ns,
			Labels:    labels,
		},
		Data: envs,
	}

	if opts.dep != nil {
		secret.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(opts.dep, appsv1.SchemeGroupVersion.WithKind("Deployment")),
		}
	}

	var newSecret *apiv1.Secret
	if oldSecret == nil {
		newSecret, err = opts.client.CoreV1().Secrets(ns).Create(ctx, &secret, metav1.CreateOptions{})

		if err == nil {
			fmt.Fprint(opts.writer, "\n")
			streamfmt.FprintlnActionf(opts.writer, "Created secret %q for process %q [version %d]", opts.secretName, opts.process, opts.version.Version())
		}
	} else {
		if secretUnchanged(&secret, oldSecret) {
			fmt.Fprint(opts.writer, "\n")
			streamfmt.FprintlnActionf(opts.writer, "No changes on secret %q for process %q [version %d]", opts.secretName, opts.process, opts.version.Version())
			return oldSecret, nil
		}

		secret.ResourceVersion = oldSecret.ResourceVersion
		newSecret, err = opts.client.CoreV1().Secrets(ns).Update(ctx, &secret, metav1.UpdateOptions{})
		if err == nil {
			fmt.Fprint(opts.writer, "\n")
			streamfmt.FprintlnActionf(opts.writer, "Updated secret %q for process %q [version %d]", opts.secretName, opts.process, opts.version.Version())
		}
	}
	return newSecret, errors.WithStack(err)
}

func cleanupAppSecret(ctx context.Context, client *ClusterClient, a *appTypes.App, secretName string) error {
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}
	err = client.CoreV1().Secrets(ns).Delete(ctx, secretName, metav1.DeleteOptions{})
	if k8sErrors.IsNotFound(err) {
		return nil
	}
	return errors.WithStack(err)
}

type createAppDeploymentOptions struct {
	client        *ClusterClient
	depName       string
	secretName    string
	oldDeployment *appsv1.Deployment
	app           *appTypes.App
	process       string
	version       appTypes.AppVersion
	replicas      int
	labels        *provision.LabelSet
	selector      map[string]string
	secret        *apiv1.Secret
}

func createAppDeployment(ctx context.Context, opts *createAppDeploymentOptions) (bool, *appsv1.Deployment, *provision.LabelSet, error) {
	realReplicas := int32(opts.replicas)
	cmdData, err := dockercommon.ContainerCmdsDataFromVersion(opts.version)
	if err != nil {
		return false, nil, nil, errors.WithStack(err)
	}
	cmds, _, err := dockercommon.LeanContainerCmds(opts.process, cmdData, opts.app)
	if err != nil {
		return false, nil, nil, errors.WithStack(err)
	}
	tenRevs := int32(10)
	webProcessName, err := opts.version.WebProcess()
	if err != nil {
		return false, nil, nil, errors.WithStack(err)
	}
	yamlData, err := opts.version.TsuruYamlData()
	if err != nil {
		return false, nil, nil, errors.WithStack(err)
	}
	processPorts, err := getProcessPortsForVersion(opts.version, opts.process)
	if err != nil {
		return false, nil, nil, errors.WithStack(err)
	}
	var hcData hcResult
	// NOTE: Here is the code that create probes for HEALTHCHECK!
	if len(yamlData.Processes) > 0 {
		var healthcheck *provTypes.TsuruYamlHealthcheck
		var startupcheck *provTypes.TsuruYamlStartupcheck
		healthcheck, startupcheck, err = yamlData.GetCheckConfigsFromProcessName(opts.process)
		if err != nil {
			return false, nil, nil, errors.WithStack(err)
		}
		hcData, err = probesFromCheckConfigs(healthcheck, startupcheck, processPorts[0].TargetPort)
		if err != nil {
			return false, nil, nil, err
		}
	} else if opts.process == webProcessName && len(processPorts) > 0 {
		hcData, err = probesFromCheckConfigs(yamlData.Healthcheck, yamlData.Startupcheck, processPorts[0].TargetPort)
		if err != nil {
			return false, nil, nil, err
		}
	}

	sleepSec := opts.client.preStopSleepSeconds(opts.app.Pool)
	terminationGracePeriod := int64(30 + sleepSec)

	var lifecycle apiv1.Lifecycle
	if sleepSec > 0 {
		lifecycle.PreStop = &apiv1.LifecycleHandler{
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
		lifecycle.PostStart = &apiv1.LifecycleHandler{
			Exec: &apiv1.ExecAction{
				Command: hookCmds,
			},
		}
	}
	maxSurge := opts.client.maxSurge(opts.app.Pool)
	maxUnavailable := opts.client.maxUnavailable(opts.app.Pool)
	disableSecrets := opts.client.disableSecrets(opts.app.Pool)

	dnsConfig := dnsConfigNdots(opts.client, opts.app)
	nodeSelector, affinity, err := defineSelectorAndAffinity(ctx, opts.app, opts.client)
	if err != nil {
		return false, nil, nil, err
	}

	_, uid := dockercommon.UserForContainer()
	overCommit, err := opts.client.OvercommitFactor(opts.app.Pool)
	if err != nil {
		return false, nil, nil, errors.WithMessage(err, "misconfigured cluster overcommit factor")
	}
	cpuOverCommit, err := opts.client.CPUOvercommitFactor(opts.app.Pool)
	if err != nil {
		return false, nil, nil, errors.WithMessage(err, "misconfigured cluster cpu overcommit factor")
	}
	poolCPUBurst, err := opts.client.CPUBurstFactor(opts.app.Pool)
	if err != nil {
		return false, nil, nil, errors.WithMessage(err, "misconfigured cluster cpu burst factor")
	}
	memoryOverCommit, err := opts.client.MemoryOvercommitFactor(opts.app.Pool)
	if err != nil {
		return false, nil, nil, errors.WithMessage(err, "misconfigured cluster memory overcommit factor")
	}

	plan, err := planForProcess(ctx, opts.app, opts.process)
	if err != nil {
		return false, nil, nil, err
	}

	resourceRequirements, err := resourceRequirements(&plan, opts.app.Pool, opts.client, requirementsFactors{
		overCommit:       overCommit,
		cpuOverCommit:    cpuOverCommit,
		poolCPUBurst:     poolCPUBurst,
		memoryOverCommit: memoryOverCommit,
	})
	if err != nil {
		return false, nil, nil, err
	}
	volumes, mounts, err := createVolumesForApp(ctx, opts.client, opts.app)
	if err != nil {
		return false, nil, nil, err
	}
	ns, err := opts.client.AppNamespace(ctx, opts.app)
	if err != nil {
		return false, nil, nil, err
	}
	deployImage := opts.version.VersionInfo().DeployImage
	pullSecrets, err := getImagePullSecrets(ctx, opts.client, ns, deployImage)
	if err != nil {
		return false, nil, nil, err
	}

	metadata := provision.GetAppMetadata(opts.app, opts.process)
	podLabels := opts.labels.PodLabels()

	for _, l := range metadata.Labels {
		opts.labels.RawLabels[l.Name] = l.Value
		podLabels[l.Name] = l.Value
	}

	annotations := map[string]string{}
	for _, annotation := range metadata.Annotations {
		annotations[annotation.Name] = annotation.Value
	}

	annotations[secretHashAnnotationKey] = generateSecretHash(opts.secret)

	depLabels := opts.labels.WithoutVersion().ToLabels()
	containerPorts := make([]apiv1.ContainerPort, len(processPorts))
	for i, port := range processPorts {
		portInt := port.TargetPort
		if portInt == 0 {
			portInt, _ = strconv.Atoi(provision.WebProcessDefaultPort())
		}
		containerPorts[i].ContainerPort = int32(portInt)
	}
	serviceLinks := false

	topologySpreadConstraints, err := topologySpreadConstraints(podLabels, opts.client.TopologySpreadConstraints(opts.app.Pool))
	if err != nil {
		return false, nil, nil, err
	}

	routers := opts.app.Routers
	conditionSet := set.Set{}
	for _, r := range routers {
		var planRouter routerTypes.PlanRouter
		_, planRouter, err = router.GetWithPlanRouter(ctx, r.Name)
		if err != nil {
			return false, nil, nil, err
		}
		for _, condition := range planRouter.ReadinessGates {
			conditionSet.Add(condition)
		}
	}
	var readinessGates []apiv1.PodReadinessGate

	if opts.process == webProcessName {
		for condition := range conditionSet {
			readinessGates = append(readinessGates, apiv1.PodReadinessGate{
				ConditionType: apiv1.PodConditionType(condition),
			})
		}
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.depName,
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
				MatchLabels: opts.selector,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: annotations,
				},
				Spec: apiv1.PodSpec{
					TopologySpreadConstraints:     topologySpreadConstraints,
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					EnableServiceLinks:            &serviceLinks,
					ImagePullSecrets:              pullSecrets,
					ServiceAccountName:            serviceAccountNameForApp(opts.app),
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: uid,
					},
					RestartPolicy:  apiv1.RestartPolicyAlways,
					NodeSelector:   nodeSelector,
					Affinity:       affinity,
					Volumes:        volumes,
					Subdomain:      headlessServiceName(opts.app, opts.process),
					ReadinessGates: readinessGates,
					DNSConfig:      dnsConfig,
					Containers: []apiv1.Container{
						{
							Name:           opts.depName,
							Image:          deployImage,
							Command:        cmds,
							Env:            appEnvs(opts.app, opts.process, opts.secretName, opts.version, disableSecrets),
							ReadinessProbe: hcData.readiness,
							LivenessProbe:  hcData.liveness,
							StartupProbe:   hcData.startup,
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
	if opts.oldDeployment == nil {
		newDep, err = opts.client.AppsV1().Deployments(ns).Create(ctx, &deployment, metav1.CreateOptions{})
	} else {
		if deploymentUnchanged(&deployment, opts.oldDeployment, realReplicas) {
			return false, opts.oldDeployment, opts.labels, nil
		}

		deployment.ResourceVersion = opts.oldDeployment.ResourceVersion
		newDep, err = opts.client.AppsV1().Deployments(ns).Update(ctx, &deployment, metav1.UpdateOptions{})
	}
	return true, newDep, opts.labels, errors.WithStack(err)
}

func deploymentUnchanged(deployment *appsv1.Deployment, oldDeployment *appsv1.Deployment, realReplicas int32) bool {
	return (deployment.ObjectMeta.Name == oldDeployment.ObjectMeta.Name &&
		deployment.ObjectMeta.Namespace == oldDeployment.ObjectMeta.Namespace &&
		apiequality.Semantic.DeepEqual(deployment.ObjectMeta.Labels, oldDeployment.ObjectMeta.Labels) &&
		annotationsUnchanged(deployment.ObjectMeta.Annotations, oldDeployment.ObjectMeta.Annotations) &&
		apiequality.Semantic.DeepDerivative(deployment.Spec, oldDeployment.Spec) &&
		oldDeployment.Status.ReadyReplicas == realReplicas &&
		oldDeployment.Status.UnavailableReplicas == 0)
}

func secretUnchanged(secret *apiv1.Secret, oldSecret *apiv1.Secret) bool {
	return (secret.ObjectMeta.Name == oldSecret.ObjectMeta.Name &&
		secret.ObjectMeta.Namespace == oldSecret.ObjectMeta.Namespace &&
		apiequality.Semantic.DeepEqual(secret.ObjectMeta.Labels, oldSecret.ObjectMeta.Labels) &&
		annotationsUnchanged(secret.ObjectMeta.Annotations, oldSecret.ObjectMeta.Annotations) &&
		reflect.DeepEqual(secret.ObjectMeta.OwnerReferences, oldSecret.ObjectMeta.OwnerReferences) &&
		secretDataEqual(secret.Data, oldSecret.Data))
}

func generateSecretHash(secret *apiv1.Secret) string {
	h := sha256.New()

	keys := []string{}
	for key := range secret.Data {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "%s:%s\n", key, string(secret.Data[key]))
	}

	hashBytes := h.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

func secretDataEqual(new, old map[string][]byte) bool {
	if len(new) != len(old) {
		return false
	}

	for key, value := range new {
		if !bytes.Equal(old[key], value) {
			return false
		}
	}

	return true
}

func annotationsUnchanged(new, old map[string]string) bool {
	for key, value := range new {
		if old[key] != value {
			return false
		}
	}

	return true
}

func appEnvs(a *appTypes.App, process string, secretName string, version appTypes.AppVersion, disableSecrets bool) []apiv1.EnvVar {
	appEnvs := EnvsForApp(a, process, version)
	envs := make([]apiv1.EnvVar, len(appEnvs))
	for i, envData := range appEnvs {
		if disableSecrets || envData.Public {
			envs[i] = apiv1.EnvVar{
				Name:  envData.Name,
				Value: strings.ReplaceAll(envData.Value, "$", "$$"),
			}
		} else {
			envs[i] = apiv1.EnvVar{
				Name: envData.Name,
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: secretName,
						},
						Key: envData.Name,
					},
				},
			}
		}
	}
	return envs
}

func appSecretEnvs(a *appTypes.App, process string, version appTypes.AppVersion) map[string][]byte {
	appEnvs := EnvsForApp(a, process, version)

	result := map[string][]byte{}

	for _, envData := range appEnvs {
		if envData.Public {
			continue
		}
		result[envData.Name] = []byte(envData.Value)
	}
	return result
}

type serviceManager struct {
	client *ClusterClient
	writer io.Writer
}

var _ servicecommon.ServiceManager = &serviceManager{}

func (m *serviceManager) CleanupServices(ctx context.Context, a *appTypes.App, deployedVersion int, preserveOldVersions bool) error {
	depGroups, err := deploymentsDataForApp(ctx, m.client, a)
	if err != nil {
		return err
	}

	type processVersionKey struct {
		process string
		version int
	}

	fmt.Fprint(m.writer, "\n")
	streamfmt.FprintlnSectionf(m.writer, "Cleaning up resources")

	baseVersion, err := baseVersionForApp(ctx, m.client, a)
	if err != nil {
		return err
	}

	processInUse := map[string]struct{}{}
	versionInUse := map[processVersionKey]struct{}{}
	multiErrors := tsuruErrors.NewMultiError()
	for _, depsData := range depGroups.versioned {
		for _, depData := range depsData {
			toKeep := (depData.isBase && depData.version == baseVersion) || preserveOldVersions
			if toKeep {
				processInUse[depData.process] = struct{}{}
				versionInUse[processVersionKey{
					process: depData.process,
					version: depData.version,
				}] = struct{}{}
				continue
			}

			streamfmt.FprintlnActionf(m.writer, "Cleaning up deployment %s", depData.dep.Name)
			err = cleanupSingleDeployment(ctx, m.client, depData.dep)
			if err != nil {
				multiErrors.Add(err)
			}

			secretName := appSecretPrefix + depData.dep.Name

			streamfmt.FprintlnActionf(m.writer, "Cleaning up secret %s", secretName)
			err = cleanupAppSecret(ctx, m.client, a, secretName)
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
		_, inUseVersion := versionInUse[processVersionKey{
			process: labels.AppProcess(),
			version: svcVersion,
		}]

		toKeep := inUseVersion || (svcVersion == 0 && inUseProcess)

		if toKeep {
			continue
		}

		streamfmt.FprintlnActionf(m.writer, "Cleaning up service %s", svc.Name)
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

		streamfmt.FprintlnActionf(m.writer, "Cleaning up PodDisruptionBudget %s", pdb.Name)
		err = m.client.PolicyV1().PodDisruptionBudgets(pdb.Namespace).Delete(ctx, pdb.Name, metav1.DeleteOptions{})
		if err != nil {
			multiErrors.Add(err)
		}
	}

	return multiErrors.ToError()
}

func (m *serviceManager) RemoveService(ctx context.Context, a *appTypes.App, process string, versionNumber int) error {
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

func (m *serviceManager) CurrentLabels(ctx context.Context, a *appTypes.App, process string, versionNumber int) (*provision.LabelSet, *int32, error) {
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

func createDeployTimeoutError(ctx context.Context, client *ClusterClient, ns string, selector map[string]string, timeout time.Duration) error {
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
		msgsStr = append(msgsStr, streamfmt.Action(m.message))
		pods = append(pods, &messages[i].pod)
	}
	var crashedUnits []string
	for u := range crashedUnitsSet {
		crashedUnits = append(crashedUnits, u)
	}

	var crashedUnitsLogs []appTypes.Applog

	crashedUnitsLogs, err = listLogsFromPods(ctx, client, ns, pods, appTypes.ListLogArgs{
		Limit: 10,
	})
	if err != nil {
		return errors.Wrap(err, "Could not get logs from crashed units")
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

func monitorDeployment(ctx context.Context, client *ClusterClient, dep *appsv1.Deployment, a *appTypes.App, processName string, w io.Writer, evtResourceVersion string, version appTypes.AppVersion) (string, error) {
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

	fmt.Fprint(w, "\n")
	streamfmt.FprintlnActionf(w, "Waiting for units to be ready [%s] [version %d]", processName, version.Version())
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
	var healthcheck *provTypes.TsuruYamlHealthcheck
	if len(tsuruYamlData.Processes) > 0 {
		healthcheck, _, err = tsuruYamlData.GetCheckConfigsFromProcessName(processName)
		if err != nil {
			return revision, errors.WithStack(err)
		}
	} else {
		healthcheck = tsuruYamlData.Healthcheck
	}
	maxWaitTimeDuration := dockercommon.DeployTimeoutFromHealthcheck(healthcheck)
	var initialCheckTimeout <-chan time.Time
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
			streamfmt.FprintlnActionf(w, "%d of %d new units created", dep.Status.UpdatedReplicas, specReplicas)
		}
		if initialCheckTimeout == nil && dep.Status.UpdatedReplicas == specReplicas {
			var allInit bool
			allInit, err = allNewPodsRunning(ctx, client, dep)
			if allInit && err == nil {
				initialCheckTimeout = time.After(maxWaitTimeDuration)
				streamfmt.FprintlnActionf(w, "waiting healthcheck on %d created units", specReplicas)
			}
		}
		readyUnits := dep.Status.UpdatedReplicas - dep.Status.UnavailableReplicas
		if oldReadyUnits != readyUnits && readyUnits >= 0 {
			streamfmt.FprintlnActionf(w, "%d of %d new units ready", readyUnits, specReplicas)
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
			streamfmt.FprintlnActionf(w, "%d old units pending termination", pendingTermination)
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
				fmt.Fprint(w, " ")
				streamfmt.FprintlnActionf(w, "%s", formatEvtMessage(msg, false))
			}
		case msg, isOpen := <-watchDepCh:
			if !isOpen {
				watchDepCh = nil
				break
			}
			fmt.Fprint(w, " ")
			streamfmt.FprintlnActionf(w, "%s", formatEvtMessage(msg, false))

		case msg, isOpen := <-watchRepCh:
			if !isOpen {
				watchRepCh = nil
				break
			}
			if isDeploymentEvent(msg, dep) {
				fmt.Fprint(w, " ")
				streamfmt.FprintlnActionf(w, "%s", formatEvtMessage(msg, false))
			}
		case <-initialCheckTimeout:
			fmt.Fprintln(w)
			streamfmt.FprintlnErrorf(w, "Healthcheck Timeout of %s exceeded", maxWaitTimeDuration.String())
			return revision, createDeployTimeoutError(ctx, client, ns, dep.Spec.Selector.MatchLabels, time.Since(t0))
		case <-timer.C:
			fmt.Fprintln(w)
			streamfmt.FprintlnErrorf(w, "Deployment Progress Timeout of %s exceeded", kubeConf.DeploymentProgressTimeout.String())
			return revision, createDeployTimeoutError(ctx, client, ns, dep.Spec.Selector.MatchLabels, time.Since(t0))
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
	fmt.Fprintln(w, streamfmt.Action("All units ready"))
	return revision, nil
}

func (m *serviceManager) DeployService(ctx context.Context, opts servicecommon.DeployServiceOpts) error {
	if m.writer == nil {
		m.writer = io.Discard
	}

	streamfmt.FprintlnSectionf(m.writer, "Updating units [%s] [version %d]", opts.ProcessName, opts.Version.Version())

	err := ensureNamespaceForApp(ctx, m.client, opts.App)
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
		Prefix: tsuruLabelPrefix,
	})

	depArgs, err := m.baseDeploymentArgs(ctx, opts.App, opts.ProcessName, opts.Labels, opts.Version, opts.PreserveVersions)
	if err != nil {
		return err
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
		processSelector := fmt.Sprintf("tsuru.io/app-name=%s, tsuru.io/app-process=%s, tsuru.io/is-routable=true", opts.App.Name, opts.ProcessName)
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

	secretName := appSecretPrefix + depArgs.name

	ensureAppSecretOptions := &ensureAppSecretOptions{
		writer:     m.writer,
		client:     m.client,
		secretName: secretName,
		app:        opts.App,
		process:    opts.ProcessName,
		version:    opts.Version,
		dep:        oldDep,
	}
	secret, err := ensureAppSecret(ctx, ensureAppSecretOptions)
	if err != nil {
		fmt.Fprintf(m.writer, "%s\n%s\n", streamfmt.Error(fmt.Sprintf("ERROR CREATING SECRET: %s", secretName)), streamfmt.Action(fmt.Sprintf(" %s ", err)))
		return err
	}

	changed, newDep, labels, err := createAppDeployment(ctx, &createAppDeploymentOptions{
		client:        m.client,
		depName:       depArgs.name,
		secretName:    secret.Name,
		oldDeployment: oldDep,
		app:           opts.App,
		process:       opts.ProcessName,
		version:       opts.Version,
		replicas:      opts.Replicas,
		labels:        opts.Labels,
		selector:      depArgs.selector,
		secret:        secret,
	})
	if err != nil {
		errs := tsuruErrors.NewMultiError()
		errs.Add(err)
		if oldDep == nil {
			err = cleanupAppSecret(ctx, m.client, opts.App, secretName)
			if err != nil {
				errs.Add(err)
			}
		}
		return errs.ToError()
	}

	if len(secret.OwnerReferences) == 0 {
		ensureAppSecretOptions.dep = newDep
		_, err = ensureAppSecret(ctx, ensureAppSecretOptions)

		if err == nil {
			fmt.Fprint(m.writer, "\n")
			streamfmt.FprintlnActionf(m.writer, "Updated secret %q ownerReferences", ensureAppSecretOptions.secretName)
		}

		if err != nil {
			fmt.Fprintf(m.writer, "%s\n%s\n", streamfmt.Error(fmt.Sprintf("ERROR UPDATING SECRET OWNERREFERENCES: %s", secretName)), streamfmt.Action(fmt.Sprintf(" %s ", err)))
			return err
		}
	}

	if changed {
		var newRevision string
		newRevision, err = monitorDeployment(ctx, m.client, newDep, opts.App, opts.ProcessName, m.writer, events.ResourceVersion, opts.Version)
		if err != nil {
			// We should only rollback if the updated deployment is a new revision.
			var rollbackErr error
			if oldDep != nil && (newRevision == "" || oldRevision == newRevision) {
				oldDep.Generation = 0
				oldDep.ResourceVersion = ""
				fmt.Fprintf(m.writer, "\n%s\n", streamfmt.Error("UPDATING BACK AFTER FAILURE"))
				_, rollbackErr = m.client.AppsV1().Deployments(ns).Update(ctx, oldDep, metav1.UpdateOptions{})
			} else if oldDep == nil {
				// We have just created the deployment, so we need to remove it
				fmt.Fprintf(m.writer, "\n%s\n", streamfmt.Error("DELETING CREATED DEPLOYMENT AFTER FAILURE"))
				rollbackErrors := tsuruErrors.NewMultiError()

				rollbackErr = m.client.AppsV1().Deployments(ns).Delete(ctx, newDep.Name, metav1.DeleteOptions{})
				if rollbackErr != nil {
					rollbackErrors.Add(rollbackErr)
				}

				rollbackErr = cleanupAppSecret(ctx, m.client, opts.App, secretName)
				if rollbackErr != nil {
					rollbackErrors.Add(rollbackErr)
				}

				rollbackErr = rollbackErrors.ToError()
			} else {
				fmt.Fprintf(m.writer, "\n%s\n", streamfmt.Error("ROLLING BACK AFTER FAILURE"))

				// This code was copied from kubernetes codebase, in the next update of version of kubectl
				// we need to move to import this library:
				// https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/kubectl/pkg/polymorphichelpers/rollback.go#L48
				rollbacker := &DeploymentRollbacker{c: m.client}
				rollbackErr = rollbacker.Rollback(ctx, m.writer, newDep)
			}
			if rollbackErr != nil {
				fmt.Fprintf(m.writer, "\n%s\n%s\n", streamfmt.Error("ERROR DURING ROLLBACK"), streamfmt.Action(fmt.Sprintf(" %s ", rollbackErr)))
			}
			if _, ok := err.(provision.ErrUnitStartup); ok {
				return err
			}
			return provision.ErrUnitStartup{Err: err}
		}
	} else {
		fmt.Fprint(m.writer, "\n")
		streamfmt.FprintlnSectionf(m.writer, "No changes on units [%s] [version %d]", opts.ProcessName, opts.Version.Version())
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
	fmt.Fprint(m.writer, "\n")
	streamfmt.FprintlnSectionf(m.writer, "Ensuring services [%s]", opts.ProcessName)
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
	baseDep  *deploymentInfo
}

func (m *serviceManager) baseDeploymentArgs(ctx context.Context, a *appTypes.App, process string, labels *provision.LabelSet, version appTypes.AppVersion, preserveVersions bool) (baseDepArgs, error) {
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
	process     string
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

func syncServiceAnnotations(app *appTypes.App, svcData *svcCreateData) {
	metadata := provision.GetAppMetadata(app, svcData.process)
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

func (m *serviceManager) ensureServices(ctx context.Context, a *appTypes.App, process string, labels *provision.LabelSet, currentVersion appTypes.AppVersion, backendCRD, preserveOldVersions bool) error {
	ns, err := m.client.AppNamespace(ctx, a)
	if err != nil {
		return err
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
			process:     process,
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
				Selector: svcData.selector,
				Ports:    svcData.ports,
				Type:     apiv1.ServiceTypeClusterIP,
			},
		}
		var isNew bool
		svc, isNew, err = mergeServices(ctx, m.client, svc)
		if err != nil {
			return err
		}
		streamfmt.FprintlnActionf(m.writer, "Service %s", svc.Name)
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

func (m *serviceManager) createHeadlessService(ctx context.Context, svcPorts []apiv1.ServicePort, ns string, a *appTypes.App, process string, labels *provision.LabelSet) error {
	enabled, err := m.client.headlessEnabled(a.Pool)
	if err != nil {
		return errors.WithStack(err)
	}
	if !enabled {
		return nil
	}
	svcName := headlessServiceName(a, process)
	streamfmt.FprintlnActionf(m.writer, "Service %s", svcName)

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
	var ports []provTypes.TsuruYamlKubernetesProcessPortConfig
	tsuruYamlData, err := version.TsuruYamlData()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	found, ports, err := getPortsFromProcessesByProcessName(tsuruYamlData.Processes, process)
	if err != nil {
		return nil, err
	}
	if found {
		return ports, nil
	}

	if tsuruYamlData.Kubernetes != nil {
		var found bool
		found, ports, err = getPortsFromTsuruYamlKubernetesByProcessName(tsuruYamlData.Kubernetes, process)
		if err != nil {
			return nil, err
		}
		if found {
			return ports, nil
		}
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

func getPortsFromProcessesByProcessName(processes []provTypes.TsuruYamlProcess, processName string) (bool, []provTypes.TsuruYamlKubernetesProcessPortConfig, error) {
	var ports []provTypes.TsuruYamlKubernetesProcessPortConfig
	portConfigFound := false

	for _, process := range processes {
		if process.Name != processName {
			continue
		}
		if portConfigFound {
			return false, nil, fmt.Errorf("duplicated process name: %s", processName)
		}
		// Only treat as port config found if Ports field has actual port configurations
		// Skip if nil or empty - let it fall through to default port logic
		if len(process.Ports) == 0 {
			continue
		}
		portConfigFound = true
		applyPortDefaults(process.Ports)
		ports = process.Ports
	}

	return portConfigFound, ports, nil
}

func getPortsFromTsuruYamlKubernetesByProcessName(kubernetes *provTypes.TsuruYamlKubernetesConfig, process string) (bool, []provTypes.TsuruYamlKubernetesProcessPortConfig, error) {
	var ports []provTypes.TsuruYamlKubernetesProcessPortConfig
	portConfigFound := false

	for _, group := range kubernetes.Groups {
		for podName, podConfig := range group {
			if podName != process {
				continue
			}
			if portConfigFound {
				return false, nil, fmt.Errorf("duplicated process name: %s", podName)
			}
			// For old format: skip only if nil, preserve empty array for "no service" semantic
			if podConfig.Ports == nil {
				continue
			}
			portConfigFound = true
			applyPortDefaults(podConfig.Ports)
			ports = podConfig.Ports

			break
		}
	}

	return portConfigFound, ports, nil
}

func applyPortDefaults(ports []provTypes.TsuruYamlKubernetesProcessPortConfig) {
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

func dnsConfigNdots(client *ClusterClient, app *appTypes.App) *apiv1.PodDNSConfig {
	dnsConfigNdots := client.dnsConfigNdots(app.Pool)
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

func deepCopyPorts(ports []apiv1.ServicePort) []apiv1.ServicePort {
	result := make([]apiv1.ServicePort, len(ports))
	for i := range ports {
		result[i] = *ports[i].DeepCopy()
	}
	return result
}
