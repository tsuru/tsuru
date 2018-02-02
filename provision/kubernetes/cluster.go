// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/cluster"
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

var clientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(conf)
}

type clusterClient struct {
	kubernetes.Interface `json:"-" bson:"-"`
	*cluster.Cluster
	restConfig *rest.Config
}

func getRestConfig(c *cluster.Cluster) (*rest.Config, error) {
	gv, err := schema.ParseGroupVersion("/v1")
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
	kubeConf := getKubeConfig()
	return &rest.Config{
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &gv,
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: scheme.Codecs},
		},
		Host: addr,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   c.CaCert,
			CertData: c.ClientCert,
			KeyData:  c.ClientKey,
		},
		Timeout:     kubeConf.APITimeout,
		BearerToken: token,
		Username:    user,
		Password:    password,
	}, nil
}

func newClusterClient(clust *cluster.Cluster) (*clusterClient, error) {
	cfg, err := getRestConfig(clust)
	if err != nil {
		return nil, err
	}
	client, err := clientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &clusterClient{
		Cluster:    clust,
		Interface:  client,
		restConfig: cfg,
	}, nil
}

func (c *clusterClient) SetTimeout(timeout time.Duration) error {
	c.restConfig.Timeout = timeout
	client, err := clientForConfig(c.restConfig)
	if err != nil {
		return err
	}
	c.Interface = client
	return nil
}

func (c *clusterClient) Namespace() string {
	if c.CustomData == nil || c.CustomData[namespaceClusterKey] == "" {
		return "default"
	}
	return c.CustomData[namespaceClusterKey]
}

func (c *clusterClient) OvercommitFactor() (int64, error) {
	if c.CustomData == nil || c.CustomData[overcommitClusterKey] == "" {
		return 1, nil
	}
	overcommit, err := strconv.Atoi(c.CustomData[overcommitClusterKey])
	return int64(overcommit), err
}

func clusterForPool(pool string) (*clusterClient, error) {
	clust, err := cluster.ForPool(provisionerName, pool)
	if err != nil {
		return nil, err
	}
	return newClusterClient(clust)
}

func allClusters() ([]*clusterClient, error) {
	clusters, err := cluster.ForProvisioner(provisionerName)
	if err != nil {
		return nil, err
	}
	clients := make([]*clusterClient, len(clusters))
	for i := range clusters {
		clients[i], err = newClusterClient(clusters[i])
		if err != nil {
			return nil, err
		}
	}
	return clients, nil
}

func forEachCluster(fn func(client *clusterClient) error) error {
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
