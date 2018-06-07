// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const (
	namespaceClusterKey  = "namespace"
	tokenClusterKey      = "token"
	userClusterKey       = "username"
	passwordClusterKey   = "password"
	overcommitClusterKey = "overcommit-factor"
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
	*cluster.Cluster
	restConfig *rest.Config
}

func getRestBaseConfig(c *cluster.Cluster) (*rest.Config, error) {
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

func getRestConfig(c *cluster.Cluster) (*rest.Config, error) {
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
	cfg.BearerToken = token
	cfg.Username = user
	cfg.Password = password
	return cfg, nil
}

func getInClusterConfig(c *cluster.Cluster) (*rest.Config, error) {
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

func NewClusterClient(clust *cluster.Cluster) (*ClusterClient, error) {
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

func (c *ClusterClient) AppNamespace(app provision.App) (string, error) {
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
	if usePoolNamespaces && len(pool) > 0 {
		return fmt.Sprintf("%s-%s", prefix, pool)
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

func (c *ClusterClient) OvercommitFactor(pool string) (int64, error) {
	if c.CustomData == nil {
		return 1, nil
	}
	overcommitConf := c.configForPool(pool, overcommitClusterKey)
	if overcommitConf == "" {
		return 1, nil
	}
	overcommit, err := strconv.Atoi(overcommitConf)
	return int64(overcommit), err
}

func (c *ClusterClient) configForPool(pool, key string) string {
	if v, ok := c.CustomData[pool+":"+key]; ok {
		return v
	}
	return c.CustomData[key]
}

func (c *ClusterClient) RestConfig() *rest.Config {
	return c.restConfig
}

func (c *ClusterClient) GetCluster() *cluster.Cluster {
	return c.Cluster
}

type clusterApp struct {
	client *ClusterClient
	apps   []provision.App
}

func clustersForApps(apps []provision.App) ([]clusterApp, error) {
	clusterClientMap := map[string]clusterApp{}
	clusterPoolMap := map[string]*cluster.Cluster{}
	var err error
	for _, a := range apps {
		poolName := a.GetPool()
		clust, inMap := clusterPoolMap[poolName]
		if !inMap {
			clust, err = cluster.ForPool(provisionerName, poolName)
			if err != nil {
				return nil, err
			}
		}
		mapItem, inMap := clusterClientMap[clust.Name]
		if !inMap {
			cli, err := NewClusterClient(clust)
			if err != nil {
				return nil, err
			}
			mapItem = clusterApp{
				client: cli,
			}
		}
		mapItem.apps = append(mapItem.apps, a)
		clusterClientMap[clust.Name] = mapItem
	}
	result := make([]clusterApp, 0, len(clusterClientMap))
	for _, v := range clusterClientMap {
		result = append(result, v)
	}
	return result, nil
}

func clusterForPool(pool string) (*ClusterClient, error) {
	clust, err := cluster.ForPool(provisionerName, pool)
	if err != nil {
		return nil, err
	}
	return NewClusterClient(clust)
}

func allClusters() ([]*ClusterClient, error) {
	clusters, err := cluster.ForProvisioner(provisionerName)
	if err != nil {
		return nil, err
	}
	clients := make([]*ClusterClient, len(clusters))
	for i := range clusters {
		clients[i], err = NewClusterClient(clusters[i])
		if err != nil {
			return nil, err
		}
	}
	return clients, nil
}

func forEachCluster(fn func(client *ClusterClient) error) error {
	clients, err := allClusters()
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
