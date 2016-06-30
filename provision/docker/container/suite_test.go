// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct {
	p      *fakeDockerProvisioner
	server *dtesting.DockerServer
	user   string
}

func (s *S) SetUpSuite(c *check.C) {
	s.user = "root"
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_container_tests")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Set("docker:cluster:mongo-database", "docker_provision_container_tests_cluster_stor")
	config.Set("docker:run-cmd:port", "8888")
	config.Set("docker:user", s.user)
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("routers:fake:type", "fakeType")
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p, err = newFakeDockerProvisioner(s.server.URL())
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *S) removeTestContainer(c *Container) error {
	routertest.FakeRouter.RemoveBackend(c.AppName)
	return c.Remove(s.p)
}

type newContainerOpts struct {
	AppName     string
	Image       string
	ProcessName string
}

func (s *S) newContainer(opts newContainerOpts, p *fakeDockerProvisioner) (*Container, error) {
	if p == nil {
		p = s.p
	}
	container := Container{
		ID:          "id",
		IP:          "10.10.10.10",
		HostPort:    "3333",
		HostAddr:    "127.0.0.1",
		ProcessName: opts.ProcessName,
		Image:       opts.Image,
		AppName:     opts.AppName,
		ExposedPort: "8888/tcp",
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	if container.ProcessName == "" {
		container.ProcessName = "web"
	}
	if container.Image == "" {
		container.Image = "tsuru/python:latest"
	}
	routertest.FakeRouter.AddBackend(container.AppName)
	routertest.FakeRouter.AddRoute(container.AppName, container.Address())
	port, err := getPort()
	if err != nil {
		return nil, err
	}
	ports := map[docker.Port]struct{}{
		docker.Port(port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        container.Image,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	err = p.Cluster().PullImage(docker.PullImageOptions{Repository: container.Image}, docker.AuthConfiguration{})
	if err != nil {
		return nil, err
	}
	_, c, err := p.Cluster().CreateContainer(docker.CreateContainerOptions{Config: &config}, net.StreamInactivityTimeout)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	coll := p.Collection()
	defer coll.Close()
	err = coll.Insert(container)
	if err != nil {
		return nil, err
	}
	return &container, nil
}
