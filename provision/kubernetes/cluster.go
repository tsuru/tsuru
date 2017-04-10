// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"time"

	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/kubernetes/cluster"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	defaultTimeout = time.Minute
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
	gv, err := unversioned.ParseGroupVersion("/v1")
	if err != nil {
		return nil, err
	}
	addr := c.Addresses[rand.Intn(len(c.Addresses))]
	return &rest.Config{
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &gv,
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		},
		Host: addr,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   c.CaCert,
			CertData: c.ClientCert,
			KeyData:  c.ClientKey,
		},
		Timeout: defaultTimeout,
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

func clusterForPool(pool string) (*clusterClient, error) {
	clust, err := cluster.ForPool(pool)
	if err != nil {
		return nil, err
	}
	return newClusterClient(clust)
}

func allClusters() ([]*clusterClient, error) {
	clusters, err := cluster.AllClusters()
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
