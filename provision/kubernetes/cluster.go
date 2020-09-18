// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const (
	namespaceClusterKey    = "namespace"
	tokenClusterKey        = "token"
	userClusterKey         = "username"
	passwordClusterKey     = "password"
	overcommitClusterKey   = "overcommit-factor"
	namespaceLabelsKey     = "namespace-labels"
	externalPolicyLocalKey = "external-policy-local"
	routerAddressLocalKey  = "router-local"
	disableHeadlessKey     = "disable-headless"
	maxSurgeKey            = "max-surge"
	maxUnavailableKey      = "max-unavailable"
	singlePoolKey          = "single-pool"
	ephemeralStorageKey    = "ephemeral-storage"
	preStopSleepKey        = "pre-stop-sleep"

	enableLogsFromAPIServerKey = "enable-logs-from-apiserver"
	defaultLogsFromAPIServer   = false

	dialTimeout  = 30 * time.Second
	tcpKeepAlive = 30 * time.Second
)

var (
	clusterHelp = map[string]string{
		namespaceClusterKey:    "Namespace used to create resources unless kubernetes:use-pool-namespaces config is enabled.",
		tokenClusterKey:        "Token used to connect to the cluster,",
		userClusterKey:         "User used to connect to the cluster.",
		passwordClusterKey:     "Password used to connect to the cluster.",
		overcommitClusterKey:   "Overcommit factor for memory resources. The requested value will be divided by this factor. This config may be prefixed with `<pool-name>:`.",
		namespaceLabelsKey:     "Extra labels added to dynamically created namespaces in the format <label1>=<value1>,<label2>=<value2>... This config may be prefixed with `<pool-name>:`.",
		externalPolicyLocalKey: "Use external policy local in created services. This is not recommended as depending on the used router it can cause downtimes during restarts. This config may be prefixed with `<pool-name>:`.",
		routerAddressLocalKey:  "Only add node addresses that contains a pod from an app to the router. This config may be prefixed with `<pool-name>:`.",
		disableHeadlessKey:     "Disable headless service creation for every app-process. This config may be prefixed with `<pool-name>:`.",
		maxSurgeKey:            "Max surge for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 100%.",
		maxUnavailableKey:      "Max unavailable for deployments rollout. This config may be prefixed with `<pool-name>:`. Defaults to 0.",
		singlePoolKey:          "Set to use entire cluster to a pool instead only designated nodes. Defaults do false.",
		ephemeralStorageKey:    fmt.Sprintf("Sets limit for ephemeral storage for created pods. This config may be prefixed with `<pool-name>:`. Defaults to %s.", defaultEphemeralStorageLimit.String()),
		preStopSleepKey:        fmt.Sprintf("Number of seconds to sleep in the preStop lifecycle hook. This config may be prefixed with `<pool-name>:`. Defaults to %d.", defaultPreStopSleepSeconds),

		enableLogsFromAPIServerKey: "Enable tsuru to request application logs from kubernetes api-server, will be enabled by default in next tsuru major version",
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
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: scheme.Codecs},
		},
		Timeout: kubeConf.APITimeout,
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
	if len(clust.Addresses) == 0 {
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

func (c *ClusterClient) AppNamespace(app appTypes.App) (string, error) {
	if app == nil {
		return c.Namespace(), nil
	}
	return c.appNamespaceByName(app.GetName())
}

func (c *ClusterClient) appNamespaceByName(appName string) (string, error) {
	tclient, err := TsuruClientForConfig(c.restConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to get client for crd")
	}
	a, err := tclient.TsuruV1().Apps(c.Namespace()).Get(appName, metav1.GetOptions{})
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
	nsPool := validKubeName(pool)
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

func (c *ClusterClient) RouterAddressLocal(pool string) (bool, error) {
	if c.CustomData == nil {
		return false, nil
	}
	routerAddressLocalConf := c.configForContext(pool, routerAddressLocalKey)
	if routerAddressLocalConf == "" {
		return false, nil
	}
	routerAddressLocal, _ := strconv.ParseBool(routerAddressLocalConf)
	if routerAddressLocal {
		return routerAddressLocal, nil
	}
	return c.ExternalPolicyLocal(pool)
}

func (c *ClusterClient) OvercommitFactor(pool string) (int64, error) {
	if c.CustomData == nil {
		return 1, nil
	}
	overcommitConf := c.configForContext(pool, overcommitClusterKey)
	if overcommitConf == "" {
		return 1, nil
	}
	overcommit, err := strconv.Atoi(overcommitConf)
	return int64(overcommit), err
}

func (c *ClusterClient) LogsFromAPIServerEnabled() bool {
	if c.CustomData == nil {
		return defaultLogsFromAPIServer
	}

	enabled, _ := strconv.ParseBool(c.CustomData[enableLogsFromAPIServerKey])
	return enabled
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

func clusterForPoolOrAny(ctx context.Context, pool string) (*ClusterClient, error) {
	clust, err := clusterForPool(ctx, pool)
	if err == nil {
		return clust, err
	}
	if err == provTypes.ErrNoCluster {
		var clusters []provTypes.Cluster
		clusters, err = servicemanager.Cluster.FindByProvisioner(ctx, provisionerName)
		if err == nil {
			return NewClusterClient(&clusters[0])
		}
	}
	return nil, err
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
