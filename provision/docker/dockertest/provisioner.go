// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

type FakeDockerProvisioner struct {
	storage    *cluster.MapStorage
	cluster    *cluster.Cluster
	authConfig docker.AuthConfiguration
	pushes     []Push
	servers    []*testing.DockerServer
	pushErrors chan error
}

func NewFakeDockerProvisioner(servers ...string) (*FakeDockerProvisioner, error) {
	var err error
	p := FakeDockerProvisioner{
		storage:    &cluster.MapStorage{},
		pushErrors: make(chan error, 10),
	}
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

func StartMultipleServersCluster() (*FakeDockerProvisioner, error) {
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		return nil, err
	}
	server2, err := testing.NewServer("localhost:0", nil, nil)
	if err != nil {
		return nil, err
	}
	otherUrl := strings.Replace(server2.URL(), "127.0.0.1", "localhost", 1)
	p, err := NewFakeDockerProvisioner(server1.URL(), otherUrl)
	if err != nil {
		return nil, err
	}
	p.servers = []*testing.DockerServer{server1, server2}
	return p, nil
}

func (p *FakeDockerProvisioner) SetAuthConfig(config docker.AuthConfiguration) {
	p.authConfig = config
}

func (p *FakeDockerProvisioner) Destroy() {
	for _, server := range p.servers {
		server.Stop()
	}
	p.servers = nil
}

func (p *FakeDockerProvisioner) Servers() []*testing.DockerServer {
	return p.servers
}

func (p *FakeDockerProvisioner) FailPush(errs ...error) {
	for _, err := range errs {
		p.pushErrors <- err
	}
}

func (p *FakeDockerProvisioner) Cluster() *cluster.Cluster {
	return p.cluster
}

func (p *FakeDockerProvisioner) Collection() *storage.Collection {
	conn, err := db.Conn()
	if err != nil {
		panic(err)
	}
	return conn.Collection("fake_docker_provisioner")
}

func (p *FakeDockerProvisioner) PushImage(name, tag string) error {
	p.pushes = append(p.pushes, Push{Name: name, Tag: tag})
	select {
	case err := <-p.pushErrors:
		return err
	default:
	}
	return nil
}

type Push struct {
	Name string
	Tag  string
}

func (p *FakeDockerProvisioner) Pushes() []Push {
	return p.pushes
}

func (p *FakeDockerProvisioner) RegistryAuthConfig() docker.AuthConfiguration {
	return p.authConfig
}
