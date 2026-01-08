// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	kedav1alpha1clientset "github.com/kedacore/keda/v2/pkg/generated/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/builder"
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
	"sigs.k8s.io/yaml"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
)

const (
	namespaceClusterKey           = "namespace"
	tokenClusterKey               = "token"
	userClusterKey                = "username"
	passwordClusterKey            = "password"
	overcommitClusterKey          = "overcommit-factor"
	cpuOvercommitClusterKey       = "cpu-overcommit-factor"
	memoryOvercommitClusterKey    = "memory-overcommit-factor"
	cpuBurstKey                   = "cpu-burst-factor"
	namespaceLabelsKey            = "namespace-labels"
	disableHeadlessKey            = "disable-headless"
	maxSurgeKey                   = "max-surge"
	maxUnavailableKey             = "max-unavailable"
	disableSecretsKey             = "disable-secrets"
	singlePoolKey                 = "single-pool"
	ephemeralStorageKey           = "ephemeral-storage"
	preStopSleepKey               = "pre-stop-sleep"
	disableDefaultNodeSelectorKey = "disable-default-node-selector"
	baseServicesAnnotations       = "base-services-annotations"
	allServicesAnnotations        = "all-services-annotations"
	registryKey                   = "registry"
	registryInsecureKey           = "registry-insecure"
	disablePlatformBuildKey       = "disable-platform-build"
	disablePDBKey                 = "disable-pdb"
	versionedServicesKey          = "enable-versioned-services"
	dockerConfigJSONKey           = "docker-config-json"
	dnsConfigNdotsKey             = "dns-config-ndots"
	buildServiceAddressKey        = "build-service-address"
	buildServiceTLSKey            = "build-service-tls"
	buildServiceTLSSkipVerify     = "build-service-tls-skip-verify"
	jobEventCreationKey           = "job-event-creation"
	topologySpreadConstraintsKey  = "topology-spread-constraints"
	debugContainerImage           = "debug-container-image"

	dialTimeout  = 30 * time.Second
	tcpKeepAlive = 30 * time.Second
)

var (
	clusterHelp = map[string]string{
		namespaceClusterKey:           "Namespace used to create resources unless kubernetes:use-pool-namespaces config is enabled.",
		tokenClusterKey:               "Token used to connect to the cluster,",
		userClusterKey:                "User used to connect to the cluster.",
		passwordClusterKey:            "Password used to connect to the cluster.",
		overcommitClusterKey:          "Overcommit factor for CPU and memory resources. The requested value will be divided by this factor. This config may be prefixed with `<pool-name>:`.",
		cpuOvercommitClusterKey:       "Overcommit factor for CPU resources. The requested value will be divided by this factor. This config may be prefixed with `<pool-name>:`.",
		memoryOvercommitClusterKey:    "Overcommit factor for Memory resources. The requested value will be divided by this factor. This config may be prefixed with `<pool-name>:`.",
		cpuBurstKey:                   "CPU burst factor, that increases the limit of resource. The requested value will be multiplied by this factor. This config may be prefixed with `<pool-name>:`.",
		namespaceLabelsKey:            "Extra labels added to dynamically created namespaces in the format <label1>=<value1>,<label2>=<value2>... This config may be prefixed with `<pool-name>:`.",
		disableHeadlessKey:            "Disable headless service creation for every app-process. This config may be prefixed with `<pool-name>:`.",
		maxSurgeKey:                   "Max surge for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 100%.",
		maxUnavailableKey:             "Max unavailable for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 0.",
		singlePoolKey:                 "Set to use entire cluster to a pool instead only designated nodes. Defaults do false.",
		ephemeralStorageKey:           fmt.Sprintf("Sets limit for ephemeral storage for created pods. This config may be prefixed with `<pool-name>:`. Defaults to %s.", defaultEphemeralStorageLimit.String()),
		preStopSleepKey:               fmt.Sprintf("Number of seconds to sleep in the preStop lifecycle hook. This config may be prefixed with `<pool-name>:`. Defaults to %d.", defaultPreStopSleepSeconds),
		disableDefaultNodeSelectorKey: "Disables the use of node selector in the cluster if enabled",
		registryKey:                   "Allow a custom registry to be used on this cluster.",
		registryInsecureKey:           "Pull and push container images to insecure registry (over plain HTTP)",
		disablePlatformBuildKey:       "Disable platform image build in cluster.",
		versionedServicesKey:          "Allow the creation of multiple services for each pair of {process, version} from the app. The default behavior creates versioned services only in a multi versioned deploy scenario.",
		dockerConfigJSONKey:           "Custom Docker config (~/.docker/config.json) to be mounted on deploy-agent container",
		disablePDBKey:                 "Disable PodDisruptionBudget for entire pool.",
		dnsConfigNdotsKey:             "Number of dots in the domain name to be used in the search list for DNS lookups. Default to uses kubernetes default value (5).",
		buildServiceAddressKey:        "Address of build service (deploy-agent v2)",
		buildServiceTLSKey:            "Whether should access Build service through TLS",
		buildServiceTLSSkipVerify:     "Whether should skip certificate chain validation",
		jobEventCreationKey:           "Enable k8s event data tracking cross-referencing with Jobs and send them to tsuru database",
		topologySpreadConstraintsKey:  "Enable topology spread constraints for apps",
		debugContainerImage:           "Image used to create debug containers (Ephemeral Containers)",
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

var KEDAClientForConfig = func(conf *rest.Config) (kedav1alpha1clientset.Interface, error) {
	return kedav1alpha1clientset.NewForConfig(conf)
}

type ClusterClient struct {
	kubernetes.Interface `json:"-" bson:"-"`
	*provTypes.Cluster
	restConfig *rest.Config
}

func getRestBaseConfig() (*rest.Config, error) {
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
		WrapTransport: tsuruNet.OtelTransport,
	}, nil
}

var randomGenerator *rand.Rand = nil

func getRestConfig(c *provTypes.Cluster) (*rest.Config, error) {
	cfg, err := getRestBaseConfig()
	if err != nil {
		return nil, err
	}
	if len(c.Addresses) == 0 {
		return nil, errors.New("no addresses for cluster")
	}

	var randPos int
	if randomGenerator == nil {
		randPos = rand.Intn(len(c.Addresses))
	} else {
		randPos = randomGenerator.Intn(len(c.Addresses))
	}

	addr := c.Addresses[randPos]
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
	restConfig.APIPath = "/api"
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
		restConfig.WrapTransport = tsuruNet.OtelTransport
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

func getInClusterConfig() (*rest.Config, error) {
	cfg, err := getRestBaseConfig()
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
		cfg, err = getInClusterConfig()
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

func (c *ClusterClient) AppNamespace(ctx context.Context, app *appTypes.App) (string, error) {
	if app == nil {
		return c.Namespace(), nil
	}
	return c.appNamespaceByName(ctx, app.Name)
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
	namespace := c.configForContext("", namespaceClusterKey)
	if namespace != "" {
		return namespace
	}
	return "tsuru"
}

func (c *ClusterClient) preStopSleepSeconds(pool string) int {
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

func (c *ClusterClient) OvercommitFactor(pool string) (float64, error) {
	overcommitConf := c.configForContext(pool, overcommitClusterKey)
	if overcommitConf == "" {
		return 1, nil
	}
	overcommit, err := strconv.ParseFloat(overcommitConf, 64)
	if err != nil {
		return 0, err
	}
	if overcommit < 1 {
		return 1, nil // Overcommit cannot be less than 1
	}
	return overcommit, nil
}

func (c *ClusterClient) CPUOvercommitFactor(pool string) (float64, error) {
	overcommitConf := c.configForContext(pool, cpuOvercommitClusterKey)
	if overcommitConf == "" {
		return 0, nil // 0 means no factor defined
	}
	overcommit, err := strconv.ParseFloat(overcommitConf, 64)
	return overcommit, err
}

func (c *ClusterClient) MemoryOvercommitFactor(pool string) (float64, error) {
	overcommitConf := c.configForContext(pool, memoryOvercommitClusterKey)
	if overcommitConf == "" {
		return 0, nil // 0 means no factor defined
	}
	overcommit, err := strconv.ParseFloat(overcommitConf, 64)
	return overcommit, err
}

func (c *ClusterClient) CPUBurstFactor(pool string) (float64, error) {
	burstConf := c.configForContext(pool, cpuBurstKey)
	if burstConf == "" {
		return 0, nil // 0 means no factor defined
	}

	burst, err := strconv.ParseFloat(burstConf, 64)
	if err != nil {
		return 0, err
	}
	if burst < 1 {
		return 0, nil // 0 means no factor defined
	}
	return burst, nil
}

func (c *ClusterClient) TopologySpreadConstraints(pool string) string {
	return c.configForContext(pool, topologySpreadConstraintsKey)
}

func (c *ClusterClient) ServiceAnnotations(key string) (map[string]string, error) {
	annotations := map[string]string{}

	annotationsRaw := c.configForContext("", key)
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

func (c *ClusterClient) headlessEnabled(pool string) (bool, error) {
	config := c.configForContext(pool, disableHeadlessKey)
	if config == "" {
		return true, nil
	}
	disableHeadless, err := strconv.ParseBool(config)
	return !disableHeadless, err
}

func (c *ClusterClient) maxSurge(pool string) intstr.IntOrString {
	defaultSurge := intstr.FromString("100%")
	maxSurge := c.configForContext(pool, maxSurgeKey)
	if maxSurge == "" {
		return defaultSurge
	}
	return intstr.Parse(maxSurge)
}

func (c *ClusterClient) maxUnavailable(pool string) intstr.IntOrString {
	defaultUnvailable := intstr.FromInt(0)
	maxUnavailable := c.configForContext(pool, maxUnavailableKey)
	if maxUnavailable == "" {
		return defaultUnvailable
	}
	return intstr.Parse(maxUnavailable)
}

func (c *ClusterClient) disableSecrets(pool string) bool {
	disableSecrets := c.configForContext(pool, disableSecretsKey)
	if disableSecrets == "" {
		return false
	}
	d, _ := strconv.ParseBool(disableSecrets)
	return d
}

func (c *ClusterClient) dnsConfigNdots(pool string) intstr.IntOrString {
	DNSConfigNdots := c.configForContext(pool, dnsConfigNdotsKey)
	if DNSConfigNdots == "" {
		return intstr.FromInt(0)
	}
	return intstr.Parse(DNSConfigNdots)
}

func (c *ClusterClient) SinglePool() (bool, error) {
	singlePool := c.configForContext("", singlePoolKey)
	if singlePool == "" {
		return false, nil
	}
	return strconv.ParseBool(singlePool)
}

func (c *ClusterClient) EnableVersionedServices() (bool, error) {
	versionedServices := c.configForContext("", versionedServicesKey)
	if versionedServices == "" {
		return false, nil
	}
	return strconv.ParseBool(versionedServices)
}

func (c *ClusterClient) EnableJobEventCreation() (bool, error) {
	jobEventCreation := c.configForContext("", jobEventCreationKey)
	if jobEventCreation == "" {
		return false, nil
	}
	return strconv.ParseBool(jobEventCreation)
}

func (c *ClusterClient) Registry() imgTypes.ImageRegistry {
	registry := c.configForContext("", registryKey)
	return imgTypes.ImageRegistry(registry)
}

func (c *ClusterClient) InsecureRegistry() bool {
	registryInsecure := c.configForContext("", registryInsecureKey)
	if registryInsecure == "" {
		return false
	}
	insecure, _ := strconv.ParseBool(registryInsecure)
	return insecure
}

func (c *ClusterClient) DisablePlatformBuild() bool {
	disablePlatformBuild := c.configForContext("", disablePlatformBuildKey)
	if disablePlatformBuild == "" {
		return false
	}
	d, _ := strconv.ParseBool(disablePlatformBuild)
	return d
}

func (c *ClusterClient) configForContext(context, key string) string {
	if v, ok := c.CustomData[context+":"+key]; ok {
		return v
	}
	if v, ok := c.CustomData[key]; ok {
		return v
	}
	data, err := config.GetString("clusters:defaults:" + key)
	if err != nil {
		return ""
	}
	return data
}

func (c *ClusterClient) RestConfig() *rest.Config {
	return c.restConfig
}

func (c *ClusterClient) GetCluster() *provTypes.Cluster {
	return c.Cluster
}

func (c *ClusterClient) disablePDB(pool string) bool {
	disablePDB := c.configForContext(pool, disablePDBKey)
	if disablePDB == "" {
		return false
	}
	d, _ := strconv.ParseBool(disablePDB)
	return d
}

func (c *ClusterClient) dockerConfigJSON() string {
	return c.configForContext("", dockerConfigJSONKey)
}

func (c *ClusterClient) BuildServiceClient(pool string) (pb.BuildClient, *grpc.ClientConn, error) {
	addr := c.configForContext(pool, buildServiceAddressKey)
	if addr == "" {
		return nil, nil, fmt.Errorf("build service address not provided: %w", builder.ErrBuildV2NotSupported)
	}

	creds := insecure.NewCredentials()

	if enableTLS, _ := strconv.ParseBool(c.configForContext(pool, buildServiceTLSKey)); enableTLS {
		var err error
		creds, err = c.buildServiceTLSCrendentials(pool, addr)
		if err != nil {
			return nil, nil, err
		}
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, nil, err
	}

	return pb.NewBuildClient(conn), conn, nil
}

func (c *ClusterClient) buildServiceTLSCrendentials(pool, addr string) (credentials.TransportCredentials, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse build service address %q as URL: %w", addr, err)
	}

	serverName, _, _ := strings.Cut(u.Host, ":") // removes the :port suffix, if any

	insecureVerify, _ := strconv.ParseBool(c.configForContext(pool, buildServiceTLSSkipVerify))

	return credentials.NewTLS(&tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: insecureVerify,
	}), nil
}

type clusterApp struct {
	client *ClusterClient
	apps   []*appTypes.App
}

func clustersForApps(ctx context.Context, apps []*appTypes.App) ([]clusterApp, error) {
	clusterClientMap := map[string]clusterApp{}
	var poolNames []string
	for _, a := range apps {
		poolNames = append(poolNames, a.Pool)
	}
	clusterPoolMap, err := servicemanager.Cluster.FindByPools(ctx, provisionerName, poolNames)
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		poolName := a.Pool
		cluster, clusterFound := clusterPoolMap[poolName]
		if !clusterFound {
			continue
		}
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

func (c *ClusterClient) DebugContainerImage() string {
	debugContainerImage := c.configForContext("", debugContainerImage)
	if debugContainerImage == "" {
		return "tsuru/netshoot"
	}
	return debugContainerImage
}
