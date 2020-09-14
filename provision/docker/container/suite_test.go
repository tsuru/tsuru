// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"context"
	"net/url"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct {
	cli     *dockercommon.PullAndCreateClient
	limiter provision.ActionLimiter
	server  *dtesting.DockerServer
	user    string
}

func (s *S) SetUpSuite(c *check.C) {
	s.user = "root"
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "docker_provision_container_tests")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("docker:cluster:mongo-database", "docker_provision_container_tests_cluster_stor")
	config.Set("docker:run-cmd:port", "8888")
	config.Set("docker:user", s.user)
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("routers:fake:type", "fakeType")
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.server, err = dtesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	s.cli = &dockercommon.PullAndCreateClient{Client: cli}
	s.limiter = &provision.LocalLimiter{}
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) removeTestContainer(c *Container) error {
	routertest.FakeRouter.RemoveBackend(context.TODO(), routertest.FakeApp{Name: c.AppName})
	return c.Remove(s.cli, s.limiter)
}

type newContainerOpts struct {
	AppName     string
	Image       string
	ProcessName string
}

func (s *S) newContainer(opts newContainerOpts, cli *dockercommon.PullAndCreateClient) (*Container, error) {
	if cli == nil {
		cli = s.cli
	}
	container := Container{
		Container: types.Container{
			ID:          "id",
			IP:          "10.10.10.10",
			HostPort:    "3333",
			HostAddr:    "127.0.0.1",
			ProcessName: opts.ProcessName,
			Image:       opts.Image,
			AppName:     opts.AppName,
			ExposedPort: "8888/tcp",
		},
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
	app := routertest.FakeApp{Name: container.AppName}
	routertest.FakeRouter.AddBackend(context.TODO(), app)
	routertest.FakeRouter.AddRoutes(context.TODO(), app, []*url.URL{container.Address()})
	ports := map[docker.Port]struct{}{
		docker.Port(provision.WebProcessDefaultPort() + "/tcp"): {},
	}
	config := docker.Config{
		Image:        container.Image,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	err := cli.PullImage(docker.PullImageOptions{Repository: container.Image}, docker.AuthConfiguration{})
	if err != nil {
		return nil, err
	}
	c, _, err := cli.PullAndCreateContainer(docker.CreateContainerOptions{Config: &config}, nil)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	return &container, nil
}
