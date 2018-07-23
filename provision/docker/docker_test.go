// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"net/url"
	"sort"

	"github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/clusterclient"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
)

type newContainerOpts struct {
	AppName         string
	Status          string
	Image           string
	ProcessName     string
	ImageCustomData map[string]interface{}
	Provisioner     *dockerProvisioner
}

func (s *S) newContainer(opts *newContainerOpts, p *dockerProvisioner) (*container.Container, error) {
	container := container.Container{
		Container: types.Container{
			ID:          "id",
			IP:          "10.10.10.10",
			HostPort:    "3333",
			HostAddr:    "127.0.0.1",
			ProcessName: "web",
			ExposedPort: "8888/tcp",
		},
	}
	if p == nil {
		p = s.p
	}
	imageName := "tsuru/python:latest"
	var customData map[string]interface{}
	if opts != nil {
		if opts.Image != "" {
			imageName = opts.Image
		}
		container.AppName = opts.AppName
		container.ProcessName = opts.ProcessName
		customData = opts.ImageCustomData
		if opts.Provisioner != nil {
			p = opts.Provisioner
		}
		container.SetStatus(p.ClusterClient(), provision.Status(opts.Status), false)
	}
	err := newFakeImage(p, imageName, customData)
	if err != nil {
		return nil, err
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	routertest.FakeRouter.AddBackend(routertest.FakeApp{Name: container.AppName})
	routertest.FakeRouter.AddRoutes(container.AppName, []*url.URL{container.Address()})
	ports := map[docker.Port]struct{}{
		docker.Port(s.port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        imageName,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	createOptions := docker.CreateContainerOptions{Config: &config}
	createOptions.Name = randomString()
	_, c, err := p.Cluster().CreateContainer(createOptions, net.StreamInactivityTimeout)
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	container.Image = imageName
	container.Name = createOptions.Name
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Collection(s.collName).Insert(&container)
	if err != nil {
		return nil, err
	}
	imageID, err := image.AppCurrentImageName(container.AppName)
	if err != nil {
		return nil, err
	}
	err = newFakeImage(p, imageID, nil)
	if err != nil {
		return nil, err
	}
	return &container, nil
}

func (s *S) removeTestContainer(c *container.Container) error {
	routertest.FakeRouter.RemoveBackend(c.AppName)
	return c.Remove(s.p.ClusterClient(), s.p.ActionLimiter())
}

func newFakeImage(p *dockerProvisioner, repo string, customData map[string]interface{}) error {
	if customData == nil {
		customData = map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "python myapp.py",
			},
		}
	}
	var buf safe.Buffer
	opts := docker.PullImageOptions{Repository: repo, OutputStream: &buf}
	err := image.SaveImageCustomData(repo, customData)
	if err != nil && !mgo.IsDup(err) {
		return err
	}
	return p.Cluster().PullImage(opts, docker.AuthConfiguration{})
}

func (s *S) TestGetContainer(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "abcdef", Type: "python"}},
		container.Container{Container: types.Container{ID: "fedajs", Type: "ruby"}},
		container.Container{Container: types.Container{ID: "wat", Type: "java"}},
	)
	container, err := s.p.GetContainer("abcdef")
	c.Assert(err, check.IsNil)
	c.Assert(container.ID, check.Equals, "abcdef")
	c.Assert(container.Type, check.Equals, "python")
	container, err = s.p.GetContainer("wut")
	c.Assert(container, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "wut")
}

func (s *S) TestGetContainers(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{Container: types.Container{ID: "abcdef", Type: "python", AppName: "something"}},
		container.Container{Container: types.Container{ID: "fedajs", Type: "python", AppName: "something"}},
		container.Container{Container: types.Container{ID: "wat", Type: "java", AppName: "otherthing"}},
	)
	containers, err := s.p.listContainersByApp("something")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	ids := []string{containers[0].ID, containers[1].ID}
	sort.Strings(ids)
	c.Assert(ids[0], check.Equals, "abcdef")
	c.Assert(ids[1], check.Equals, "fedajs")
	containers, err = s.p.listContainersByApp("otherthing")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].ID, check.Equals, "wat")
	containers, err = s.p.listContainersByApp("unknown")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
}

func (s *S) TestGetImageFromAppPlatform(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	img, err := image.GetBuildImage(app)
	c.Assert(err, check.IsNil)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/python:v1", repoNamespace))
}

func (s *S) TestGetImageAppWhenDeployIsMultipleOf10(c *check.C) {
	app := &app.App{Name: "app1", Platform: "python", Deploys: 20}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	cont := container.Container{Container: types.Container{ID: "bleble", Type: app.Platform, AppName: app.Name, Image: "tsuru/app1"}}
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Insert(cont)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	img, err := image.GetBuildImage(app)
	c.Assert(err, check.IsNil)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/%s:v1", repoNamespace, app.Platform))
}

func (s *S) TestGetImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	s.mockService.PlatformImage.OnCurrentImage = func(name string) (string, error) {
		return "localhost:3030/tsuru/" + name + ":v1", nil
	}
	img, err := image.GetBuildImage(app)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	expected := fmt.Sprintf("localhost:3030/%s/python:v1", repoNamespace)
	c.Assert(img, check.Equals, expected)
}

func (s *S) TestStart(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:v1", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageID, err := image.GetBuildImage(app)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app)
	var buf bytes.Buffer
	cont, err := s.p.start(&container.Container{Container: types.Container{ProcessName: "web"}}, app, imageID, &buf, "")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, imageID)
	c.Assert(cont2.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestStartStoppedContainer(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	cont.Status = provision.StatusStopped.String()
	err = newFakeImage(s.p, "tsuru/python:v1", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageID, err := image.GetBuildImage(app)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(app)
	var buf bytes.Buffer
	cont, err = s.p.start(cont, app, imageID, &buf, "")
	c.Assert(err, check.IsNil)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, imageID)
	c.Assert(cont2.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestProvisionerGetCluster(c *check.C) {
	config.Set("docker:cluster:redis-server", "127.0.0.1:6379")
	defer config.Unset("docker:cluster:redis-server")
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	clus := p.Cluster()
	c.Assert(clus, check.NotNil)
	currentNodes, err := clus.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(currentNodes, check.HasLen, 0)
	c.Assert(p.scheduler, check.NotNil)
}

func (s *S) TestBuildClusterStorage(c *check.C) {
	defer config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	defer config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Unset("docker:cluster:mongo-url")
	_, err := buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Unset("docker:cluster:mongo-database")
	_, err = buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:storage", "xxxx")
}

func (s *S) TestGetClient(c *check.C) {
	p := &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, p.storage, "")
	c.Assert(err, check.IsNil)
	client, err := p.GetClient(nil)
	c.Assert(err, check.IsNil)
	clusterClient := client.(*clusterclient.ClusterClient)
	c.Assert(clusterClient.Cluster, check.Equals, p.Cluster())
	c.Assert(clusterClient.Limiter, check.Equals, p.ActionLimiter())
	c.Assert(clusterClient.Collection, check.NotNil)
}
