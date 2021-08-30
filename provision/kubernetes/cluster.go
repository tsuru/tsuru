// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	provTypes "github.com/tsuru/tsuru/types/provision"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	vpaclientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	backendConfigClientSet "k8s.io/ingress-gce/pkg/backendconfig/client/clientset/versioned"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	namespaceClusterKey           = "namespace"
	tokenClusterKey               = "token"
	userClusterKey                = "username"
	passwordClusterKey            = "password"
	overcommitClusterKey          = "overcommit-factor"
	namespaceLabelsKey            = "namespace-labels"
	externalPolicyLocalKey        = "external-policy-local"
	disableHeadlessKey            = "disable-headless"
	maxSurgeKey                   = "max-surge"
	maxUnavailableKey             = "max-unavailable"
	singlePoolKey                 = "single-pool"
	ephemeralStorageKey           = "ephemeral-storage"
	preStopSleepKey               = "pre-stop-sleep"
	disableDefaultNodeSelectorKey = "disable-default-node-selector"
	disableNodeContainers         = "disable-node-containers"
	disableUnitRegisterCmdKey     = "disable-unit-register"
	buildPlanKey                  = "build-plan"
	buildPlanSideCarKey           = "build-plan-sidecar"
	baseServicesAnnotations       = "base-services-annotations"
	enableLogsFromAPIServerKey    = "enable-logs-from-apiserver"
	registryKey                   = "registry"
	sidecarRegistryKey            = "sidecar-registry"
	buildServiceAccountKey        = "build-service-account"
	disablePlatformBuildKey       = "disable-platform-build"
	defaultLogsFromAPIServer      = false

	dialTimeout  = 30 * time.Second
	tcpKeepAlive = 30 * time.Second
)

var (
	clusterHelp = map[string]string{
		namespaceClusterKey:           "Namespace used to create resources unless kubernetes:use-pool-namespaces config is enabled.",
		tokenClusterKey:               "Token used to connect to the cluster,",
		userClusterKey:                "User used to connect to the cluster.",
		passwordClusterKey:            "Password used to connect to the cluster.",
		overcommitClusterKey:          "Overcommit factor for memory resources. The requested value will be divided by this factor. This config may be prefixed with `<pool-name>:`.",
		namespaceLabelsKey:            "Extra labels added to dynamically created namespaces in the format <label1>=<value1>,<label2>=<value2>... This config may be prefixed with `<pool-name>:`.",
		externalPolicyLocalKey:        "Use external policy local in created services. This is not recommended as depending on the used router it can cause downtimes during restarts. This config may be prefixed with `<pool-name>:`.",
		disableHeadlessKey:            "Disable headless service creation for every app-process. This config may be prefixed with `<pool-name>:`.",
		maxSurgeKey:                   "Max surge for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 100%.",
		maxUnavailableKey:             "Max unavailable for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 0.",
		singlePoolKey:                 "Set to use entire cluster to a pool instead only designated nodes. Defaults do false.",
		ephemeralStorageKey:           fmt.Sprintf("Sets limit for ephemeral storage for created pods. This config may be prefixed with `<pool-name>:`. Defaults to %s.", defaultEphemeralStorageLimit.String()),
		preStopSleepKey:               fmt.Sprintf("Number of seconds to sleep in the preStop lifecycle hook. This config may be prefixed with `<pool-name>:`. Defaults to %d.", defaultPreStopSleepSeconds),
		disableDefaultNodeSelectorKey: "Disables the use of node selector in the cluster if enabled",
		buildPlanKey:                  "Name of the plan to be used during pod build, this is required if the pool namespace has ResourceQuota set",
		buildPlanSideCarKey:           "Name of sidecar plan to be used during pod build. Defaults same as build-plan if omitted",
		enableLogsFromAPIServerKey:    "Enable tsuru to request application logs from kubernetes api-server, will be enabled by default in next tsuru major version",
		registryKey:                   "Allow a custom registry to be used on this cluster.",
		buildServiceAccountKey:        "Custom service account used in build containers.",
		disablePlatformBuildKey:       "Disable platform image build in cluster.",
		sidecarRegistryKey:            "Override for deploy sidecar image registry.",
	}
)

var ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(conf)
}

var ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
	return apiextensionsclientset.NewForConfig(conf)
}

var TsuruClientForConfig = func(conf *rest.Config) (tsuruv1clientset.Interface, error) {
	return tsuruv1clientset.NewForConfig(conf)
}

var VPAClientForConfig = func(conf *rest.Config) (vpaclientset.Interface, error) {
	return vpaclientset.NewForConfig(conf)
}

var BackendConfigClientForConfig = func(conf *rest.Config) (backendConfigClientSet.Interface, error) {
	return backendConfigClientSet.NewForConfig(conf)
}

var MetricsClientForConfig = func(conf *rest.Config) (metricsclientset.Interface, error) {
	return metricsclientset.NewForConfig(conf)
}

type ClusterClient struct {
	kubernetes.Interface `json:"-" bson:"-"`
	*provTypes.Cluster
	restConfig *rest.Config
}

func getRestBaseConfig(c *provTypes.Cluster) (*rest.Config, error) {
	gv, err := schema.ParseGroupVersion("/v1")
	if err != nil {
		return nil, err
	}
	kubeConf := getKubeConfig()
	return &rest.Config{
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &gv,
			NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs},
		},
		Timeout:       kubeConf.APITimeout,
		WrapTransport: tsuruNet.OpentracingTransport,
	}, nil
}

func getRestConfig(c *provTypes.Cluster) (*rest.Config, error) {
	cfg, err := getRestBaseConfig(c)
	if err != nil {
		return nil, err
	}
	if len(c.Addresses) == 0 {
		return nil, errors.New("no addresses for cluster")
	}
	addr := c.Addresses[rand.Intn(len(c.Addresses))]
	token, user, password := "", "", ""
	if c.CustomData != nil {
		token = c.CustomData[tokenClusterKey]
		user = c.CustomData[userClusterKey]
		password = c.CustomData[passwordClusterKey]
	}
	cfg.Host = addr
	cfg.TLSClientConfig = rest.TLSClientConfig{
		CAData:   c.CaCert,
		CertData: c.ClientCert,
		KeyData:  c.ClientKey,
	}
	if user != "" && password != "" {
		cfg.Username = user
		cfg.Password = password
	} else {
		cfg.BearerToken = token
	}
	cfg.Dial = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: tcpKeepAlive,
	}).DialContext
	return cfg, nil
}

func getRestConfigByKubeConfig(cluster *provTypes.Cluster) (*rest.Config, error) {
	gv, err := schema.ParseGroupVersion("/v1")
	if err != nil {
		return nil, err
	}

	cliCfg := clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: cluster.Name,
		Clusters: map[string]*clientcmdapi.Cluster{
			cluster.Name: &cluster.KubeConfig.Cluster,
		},
		Contexts: map[string]*clientcmdapi.Context{
			cluster.Name: {
				Cluster:  cluster.Name,
				AuthInfo: cluster.Name,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			cluster.Name: &cluster.KubeConfig.AuthInfo,
		},
	}
	restConfig, err := clientcmd.NewNonInteractiveClientConfig(cliCfg, cluster.Name, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeConf := getKubeConfig()
	restConfig.ContentConfig = rest.ContentConfig{
		GroupVersion:         &gv,
		NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs},
	}
	restConfig.Timeout = kubeConf.APITimeout

	proxyURL, err := url.Parse(cluster.HTTPProxy)
	if err != nil {
		return nil, err
	}

	if cluster.HTTPProxy == "" {
		restConfig.WrapTransport = tsuruNet.OpentracingTransport
	} else {
		restConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			transport, ok := rt.(*http.Transport)

			if !ok {
				log.Errorf("Could not apply patch to current transport, creating new one")
				return &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
				}
			}
			transport.Proxy = http.ProxyURL(proxyURL)
			return transport
		}
	}
	return restConfig, nil
}

func getInClusterConfig(c *provTypes.Cluster) (*rest.Config, error) {
	cfg, err := getRestBaseConfig(c)
	if err != nil {
		return nil, err
	}
	inClusterCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	cfg.Host = inClusterCfg.Host
	cfg.BearerToken = inClusterCfg.BearerToken
	cfg.TLSClientConfig = inClusterCfg.TLSClientConfig
	return cfg, nil
}

func NewClusterClient(clust *provTypes.Cluster) (*ClusterClient, error) {
	var cfg *rest.Config
	var err error
	if clust.KubeConfig != nil {
		cfg, err = getRestConfigByKubeConfig(clust)
	} else if clust.Local || len(clust.Addresses) == 0 {
		cfg, err = getInClusterConfig(clust)
	} else {
		cfg, err = getRestConfig(clust)
	}
	if err != nil {
		return nil, err
	}
	client, err := ClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &ClusterClient{
		Cluster:    clust,
		Interface:  client,
		restConfig: cfg,
	}, nil
}

func (c *ClusterClient) SetTimeout(timeout time.Duration) error {
	c.restConfig.Timeout = timeout
	client, err := ClientForConfig(c.restConfig)
	if err != nil {
		return err
	}
	c.Interface = client
	return nil
}

func (c *ClusterClient) AppNamespace(ctx context.Context, app appTypes.App) (string, error) {
	if app == nil {
		return c.Namespace(), nil
	}
	return c.appNamespaceByName(ctx, app.GetName())
}

func (c *ClusterClient) appNamespaceByName(ctx context.Context, appName string) (string, error) {
	tclient, err := TsuruClientForConfig(c.restConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to get client for crd")
	}
	a, err := tclient.TsuruV1().Apps(c.Namespace()).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get app custom resource")
	}
	return a.Spec.NamespaceName, nil
}

func (c *ClusterClient) PoolNamespace(pool string) string {
	usePoolNamespaces, _ := config.GetBool("kubernetes:use-pool-namespaces")
	prefix := "default"
	if usePoolNamespaces {
		prefix = "tsuru"
	}
	if c.CustomData != nil && c.CustomData[namespaceClusterKey] != "" {
		prefix = c.CustomData[namespaceClusterKey]
	}
	// Replace invalid characters from the pool name, to keep compatibility with older tsuru versions
	nsPool := provision.ValidKubeName(pool)
	if usePoolNamespaces && len(nsPool) > 0 {
		return fmt.Sprintf("%s-%s", prefix, nsPool)
	}
	return prefix
}

// Namespace returns the namespace to be used by Custom Resources
func (c *ClusterClient) Namespace() string {
	if c.CustomData != nil && c.CustomData[namespaceClusterKey] != "" {
		return c.CustomData[namespaceClusterKey]
	}
	return "tsuru"
}

func (c *ClusterClient) preStopSleepSeconds(pool string) int {
	if c.CustomData == nil {
		return defaultPreStopSleepSeconds
	}
	sleepRaw := c.configForContext(pool, preStopSleepKey)
	if sleepRaw == "" {
		return defaultPreStopSleepSeconds
	}
	sleep, err := strconv.Atoi(sleepRaw)
	if err != nil {
		return defaultPreStopSleepSeconds
	}
	return sleep
}

func (c *ClusterClient) ephemeralStorage(pool string) (resource.Quantity, error) {
	if c.CustomData == nil {
		return defaultEphemeralStorageLimit, nil
	}
	ephemeralRaw := c.configForContext(pool, ephemeralStorageKey)
	if ephemeralRaw == "" {
		return defaultEphemeralStorageLimit, nil
	}
	quantity, err := resource.ParseQuantity(ephemeralRaw)
	if err != nil {
		return defaultEphemeralStorageLimit, nil
	}
	return quantity, nil
}

func (c *ClusterClient) ExternalPolicyLocal(pool string) (bool, error) {
	if c.CustomData == nil {
		return false, nil
	}
	externalPolicyLocalConf := c.configForContext(pool, externalPolicyLocalKey)
	if externalPolicyLocalConf == "" {
		return false, nil
	}
	externalPolicyLocal, err := strconv.ParseBool(externalPolicyLocalConf)
	return externalPolicyLocal, err
}

func (c *ClusterClient) OvercommitFactor(pool string) (float64, error) {
	if c.CustomData == nil {
		return 1, nil
	}
	overcommitConf := c.configForContext(pool, overcommitClusterKey)
	if overcommitConf == "" {
		return 1, nil
	}
	overcommit, err := strconv.ParseFloat(overcommitConf, 64)
	return overcommit, err
}

func (c *ClusterClient) LogsFromAPIServerEnabled() bool {
	if c.CustomData == nil {
		return defaultLogsFromAPIServer
	}

	enabled, _ := strconv.ParseBool(c.CustomData[enableLogsFromAPIServerKey])
	return enabled
}

func (c *ClusterClient) BaseServiceAnnotations() (map[string]string, error) {
	annotations := map[string]string{}
	if c.CustomData == nil {
		return nil, nil
	}

	annotationsRaw := c.CustomData[baseServicesAnnotations]
	if annotationsRaw == "" {
		return nil, nil
	}

	err := yaml.Unmarshal([]byte(annotationsRaw), &annotations)
	if err != nil {
		return nil, err
	}

	return annotations, nil
}

func (c *ClusterClient) namespaceLabels(ns string) (map[string]string, error) {
	labels := map[string]string{
		"name": ns,
	}
	if c.CustomData == nil {
		return labels, nil
	}
	nsLabelsConf := c.configForContext(ns, namespaceLabelsKey)
	if nsLabelsConf == "" {
		return labels, nil
	}
	labelsRaw := strings.Split(nsLabelsConf, ",")
	for _, l := range labelsRaw {
		parts := strings.Split(l, "=")
		if len(parts) != 2 {
			continue
		}
		labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return labels, nil
}

func (c *ClusterClient) unitRegisterCmdEnabled() bool {
	if c.CustomData == nil {
		return true
	}
	config := c.CustomData[disableUnitRegisterCmdKey]
	if config == "" {
		return true
	}
	disableUnitRegister, _ := strconv.ParseBool(config)
	return !disableUnitRegister
}

func (c *ClusterClient) headlessEnabled(pool string) (bool, error) {
	if c.CustomData == nil {
		return true, nil
	}
	config := c.configForContext(pool, disableHeadlessKey)
	if config == "" {
		return true, nil
	}
	disableHeadless, err := strconv.ParseBool(config)
	return !disableHeadless, err
}

func (c *ClusterClient) maxSurge(pool string) intstr.IntOrString {
	defaultSurge := intstr.FromString("100%")
	if c.CustomData == nil {
		return defaultSurge
	}
	maxSurge := c.configForContext(pool, maxSurgeKey)
	if maxSurge == "" {
		return defaultSurge
	}
	return intstr.Parse(maxSurge)
}

func (c *ClusterClient) maxUnavailable(pool string) intstr.IntOrString {
	defaultUnvailable := intstr.FromInt(0)
	if c.CustomData == nil {
		return defaultUnvailable
	}
	maxUnavailable := c.configForContext(pool, maxUnavailableKey)
	if maxUnavailable == "" {
		return defaultUnvailable
	}
	return intstr.Parse(maxUnavailable)
}

func (c *ClusterClient) SinglePool() (bool, error) {
	if c.CustomData == nil {
		return false, nil
	}
	singlePool, ok := c.CustomData[singlePoolKey]
	if singlePool == "" || !ok {
		return false, nil
	}
	return strconv.ParseBool(singlePool)
}

func (c *ClusterClient) registry() imgTypes.ImageRegistry {
	if c.CustomData == nil {
		return ""
	}
	return imgTypes.ImageRegistry(c.CustomData[registryKey])
}

func (c *ClusterClient) buildServiceAccount(a provision.App) string {
	if c.CustomData == nil && a != nil {
		return serviceAccountNameForApp(a)
	}
	sa, ok := c.CustomData[buildServiceAccountKey]
	if !ok && a != nil {
		return serviceAccountNameForApp(a)
	}
	return sa
}

func (c *ClusterClient) DisablePlatformBuild() bool {
	if c.CustomData == nil {
		return false
	}
	disable, ok := c.CustomData[disablePlatformBuildKey]
	if !ok {
		return false
	}
	d, _ := strconv.ParseBool(disable)
	return d
}

func (c *ClusterClient) sideCarImage(imgName string) string {
	if c.CustomData == nil {
		return imgName
	}
	newRepo, ok := c.CustomData[sidecarRegistryKey]
	if !ok {
		return imgName
	}
	_, img, tag := image.ParseImageParts(imgName)
	if tag == "" {
		tag = "latest"
	}
	fullImg := fmt.Sprintf("%s:%s", img, tag)
	if newRepo != "" {
		fullImg = fmt.Sprintf("%s/%s", newRepo, fullImg)
	}
	return fullImg
}

func (c *ClusterClient) deploySidecarImage() string {
	conf := getKubeConfig()
	return c.sideCarImage(conf.deploySidecarImage)
}

func (c *ClusterClient) deployInspectImage() string {
	conf := getKubeConfig()
	return c.sideCarImage(conf.deployInspectImage)
}

func (c *ClusterClient) configForContext(context, key string) string {
	if v, ok := c.CustomData[context+":"+key]; ok {
		return v
	}
	return c.CustomData[key]
}

func (c *ClusterClient) RestConfig() *rest.Config {
	return c.restConfig
}

func (c *ClusterClient) GetCluster() *provTypes.Cluster {
	return c.Cluster
}

type clusterApp struct {
	client *ClusterClient
	apps   []provision.App
}

func clustersForApps(ctx context.Context, apps []provision.App) ([]clusterApp, error) {
	clusterClientMap := map[string]clusterApp{}
	var poolNames []string
	for _, a := range apps {
		poolNames = append(poolNames, a.GetPool())
	}
	clusterPoolMap, err := servicemanager.Cluster.FindByPools(ctx, provisionerName, poolNames)
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		poolName := a.GetPool()
		cluster := clusterPoolMap[poolName]
		mapItem, inMap := clusterClientMap[cluster.Name]
		if !inMap {
			cli, err := NewClusterClient(&cluster)
			if err != nil {
				return nil, err
			}
			mapItem = clusterApp{
				client: cli,
			}
		}
		mapItem.apps = append(mapItem.apps, a)
		clusterClientMap[cluster.Name] = mapItem
	}
	result := make([]clusterApp, 0, len(clusterClientMap))
	for _, v := range clusterClientMap {
		result = append(result, v)
	}
	return result, nil
}

func clusterForPool(ctx context.Context, pool string) (*ClusterClient, error) {
	clust, err := servicemanager.Cluster.FindByPool(ctx, provisionerName, pool)
	if err != nil {
		return nil, err
	}
	return NewClusterClient(clust)
}

func allClusters(ctx context.Context) ([]*ClusterClient, error) {
	clusters, err := servicemanager.Cluster.FindByProvisioner(ctx, provisionerName)
	if err != nil {
		return nil, err
	}
	clients := make([]*ClusterClient, len(clusters))
	for i := range clusters {
		clients[i], err = NewClusterClient(&clusters[i])
		if err != nil {
			return nil, err
		}
	}
	return clients, nil
}

func forEachCluster(ctx context.Context, fn func(client *ClusterClient) error) error {
	clients, err := allClusters(ctx)
	if err != nil {
		return err
	}
	errors := tsuruErrors.NewMultiError()
	for _, c := range clients {
		err = fn(c)
		if err != nil {
			errors.Add(err)
		}
	}
	if errors.Len() > 0 {
		return errors
	}
	return nil
}
