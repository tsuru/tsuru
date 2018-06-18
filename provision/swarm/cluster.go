// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"crypto/x509"
	"math/rand"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/provision"
)

type clusterClient struct {
	*docker.Client
	*provision.Cluster
}

func clusterHasTLS(clust *provision.Cluster) bool {
	return len(clust.ClientCert) != 0 && len(clust.ClientKey) != 0
}

func tlsConfigForCluster(clust *provision.Cluster) (*tls.Config, error) {
	if !clusterHasTLS(clust) {
		return nil, nil
	}
	tlsCert, err := tls.X509KeyPair(clust.ClientCert, clust.ClientKey)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(clust.CaCert) {
		return nil, errors.New("could not add RootCA pem")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
	}, nil
}

func newClusterClient(clust *provision.Cluster) (*clusterClient, error) {
	tlsConfig, err := tlsConfigForCluster(clust)
	if err != nil {
		return nil, err
	}
	addr := clust.Addresses[rand.Intn(len(clust.Addresses))]
	client, err := newClient(addr, tlsConfig)
	if err != nil {
		return nil, err
	}
	return &clusterClient{
		Client:  client,
		Cluster: clust,
	}, nil
}

func clusterForPool(pool string) (*clusterClient, error) {
	clust, err := servicemanager.Cluster.FindByPool(provisionerName, pool)
	if err != nil {
		return nil, err
	}
	return newClusterClient(clust)
}

func allClusters() ([]*clusterClient, error) {
	clusters, err := servicemanager.Cluster.FindByProvisioner(provisionerName)
	if err != nil {
		return nil, err
	}
	clients := make([]*clusterClient, len(clusters))
	for i := range clusters {
		clients[i], err = newClusterClient(&clusters[i])
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
