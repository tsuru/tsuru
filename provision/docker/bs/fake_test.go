// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
)

type fakeDockerProvisioner struct {
	storage cluster.Storage
	cluster *cluster.Cluster
}

func newFakeDockerProvisioner(servers ...string) (*fakeDockerProvisioner, error) {
	var err error
	p := fakeDockerProvisioner{storage: &cluster.MapStorage{}}
	nodes := make([]cluster.Node, len(servers))
	for i, server := range servers {
		nodes[i] = cluster.Node{Address: server}
	}
	p.cluster, err = cluster.New(nil, p.storage, nodes...)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *fakeDockerProvisioner) Cluster() *cluster.Cluster {
	return p.cluster
}

func (p *fakeDockerProvisioner) RegistryAuthConfig() docker.AuthConfiguration {
	return docker.AuthConfiguration{}
}
