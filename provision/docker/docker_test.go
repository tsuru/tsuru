// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
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
		container.SetStatus(p, provision.Status(opts.Status), false)
	}
	err := newFakeImage(p, imageName, customData)
	if err != nil {
		return nil, err
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	routertest.FakeRouter.AddBackend(container.AppName)
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
	return c.Remove(s.p)
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
	img := image.GetBuildImage(app)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/python:latest", repoNamespace))
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
	img := image.GetBuildImage(app)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/%s:latest", repoNamespace, app.Platform))
}

func (s *S) TestGetImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	img := image.GetBuildImage(app)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	expected := fmt.Sprintf("localhost:3030/%s/python:latest", repoNamespace)
	c.Assert(img, check.Equals, expected)
}

func (s *S) TestStart(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageID := image.GetBuildImage(app)
	routertest.FakeRouter.AddBackend(app.GetName())
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
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageID := image.GetBuildImage(app)
	routertest.FakeRouter.AddBackend(app.GetName())
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

func (s *S) TestPushImage(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL()})
	c.Assert(err, check.IsNil)
	err = newFakeImage(&p, "localhost:3030/base/img", nil)
	c.Assert(err, check.IsNil)
	err = p.PushImage("localhost:3030/base/img", "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[2].URL.RawQuery, check.Equals, "")
	err = newFakeImage(&p, "localhost:3030/base/img:v2", nil)
	c.Assert(err, check.IsNil)
	err = p.PushImage("localhost:3030/base/img", "v2")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 6)
	c.Assert(requests[3].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[4].URL.Path, check.Equals, "/images/localhost:3030/base/img:v2/json")
	c.Assert(requests[5].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[5].URL.RawQuery, check.Equals, "tag=v2")
}

func (s *S) TestPushImageAuth(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	config.Set("docker:registry-auth:email", "me@company.com")
	config.Set("docker:registry-auth:username", "myuser")
	config.Set("docker:registry-auth:password", "mypassword")
	defer config.Unset("docker:registry")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL()})
	c.Assert(err, check.IsNil)
	err = newFakeImage(&p, "localhost:3030/base/img", nil)
	c.Assert(err, check.IsNil)
	err = p.PushImage("localhost:3030/base/img", "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[2].URL.RawQuery, check.Equals, "")
	auth := requests[2].Header.Get("X-Registry-Auth")
	var providedAuth docker.AuthConfiguration
	data, err := base64.StdEncoding.DecodeString(auth)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &providedAuth)
	c.Assert(err, check.IsNil)
	c.Assert(providedAuth.ServerAddress, check.Equals, "localhost:3030")
	c.Assert(providedAuth.Email, check.Equals, "me@company.com")
	c.Assert(providedAuth.Username, check.Equals, "myuser")
	c.Assert(providedAuth.Password, check.Equals, "mypassword")
}

func (s *S) TestPushImageNoRegistry(c *check.C) {
	var request *http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		request = r
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	err = s.p.PushImage("localhost:3030/base", "")
	c.Assert(err, check.IsNil)
	c.Assert(request, check.IsNil)
}

func (s *S) TestBuildClusterStorage(c *check.C) {
	defer config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	defer config.Set("docker:cluster:mongo-database", "docker_provision_tests_cluster_stor")
	config.Unset("docker:cluster:mongo-url")
	_, err := buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:mongo-url", "127.0.0.1:27017")
	config.Unset("docker:cluster:mongo-database")
	_, err = buildClusterStorage()
	c.Assert(err, check.ErrorMatches, ".*docker:cluster:{mongo-url,mongo-database} must be set.")
	config.Set("docker:cluster:storage", "xxxx")
}

func (s *S) TestGetNodeByHost(c *check.C) {
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	nodes := []cluster.Node{{
		Address: "http://h1:80",
	}, {
		Address: "http://h2:90",
	}, {
		Address: "http://h3",
	}, {
		Address: "h4",
	}, {
		Address: "h5:30123",
	}}
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "", nodes...)
	c.Assert(err, check.IsNil)
	tests := [][]string{
		{"h1", nodes[0].Address},
		{"h2", nodes[1].Address},
		{"h3", nodes[2].Address},
		{"h4", nodes[3].Address},
		{"h5", nodes[4].Address},
	}
	for _, t := range tests {
		var n cluster.Node
		n, err = p.GetNodeByHost(t[0])
		c.Assert(err, check.IsNil)
		c.Assert(n.Address, check.DeepEquals, t[1])
	}
	_, err = p.GetNodeByHost("h6")
	c.Assert(err, check.ErrorMatches, `node with host "h6" not found`)
}

func (s *S) TestGetDockerClientNoSuchNode(c *check.C) {
	p := &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, p.storage, "")
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test-docker-client"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "myapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
		Pool:      "test-docker-client",
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	_, err = p.GetDockerClient(&a)
	c.Assert(err, check.ErrorMatches, "No such node in storage")
}

func (s *S) TestGetDockerClientWithApp(c *check.C) {
	p := &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	nodes := []cluster.Node{
		{
			Address:  "http://h1:80",
			Metadata: map[string]string{"pool": "test-docker-client"},
		},
	}
	p.cluster, err = cluster.New(nil, p.storage, "", nodes...)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test-docker-client"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "myapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Router:    "fake",
		Pool:      "test-docker-client",
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	client, err := p.GetDockerClient(&a)
	c.Assert(err, check.IsNil)
	c.Assert(client.(*dbAwareClient).Endpoint(), check.Equals, nodes[0].Address)
}

func (s *S) TestGetDockerClientWithoutApp(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "cont1", AppName: a1.Name, ProcessName: "web"}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "cont2", AppName: a1.Name, ProcessName: "web"}}
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Insert(cont1, cont2)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	err = pool.AddPool(pool.AddPoolOptions{Name: p.Name})
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	nodes := []cluster.Node{
		{Address: "http://h1:80"},
		{Address: "http://h2:80"},
		{Address: "http://h3:80"},
	}
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.cluster, err = cluster.New(&scheduler, s.p.storage, "")
	c.Assert(err, check.IsNil)
	n, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.HasLen, 1)
	err = s.p.cluster.Unregister(n[0].Address)
	c.Assert(err, check.IsNil)
	for _, node := range nodes {
		err = s.p.cluster.Register(cluster.Node{
			Address:  node.Address,
			Metadata: map[string]string{"pool": p.Name},
		})
		c.Assert(err, check.IsNil)
	}
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(s.p.cluster, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: cont1.ProcessName})
	c.Assert(err, check.IsNil)
	client, err := s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	c.Assert(client.(*dbAwareClient).Endpoint(), check.Equals, nodes[1].Address)
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	_, err = scheduler.Schedule(s.p.cluster, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: cont2.ProcessName})
	c.Assert(err, check.IsNil)
	client, err = s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	c.Assert(client.(*dbAwareClient).Endpoint(), check.Equals, nodes[2].Address)
}

func (s *S) TestGetDockerClientWithoutAppOrNode(c *check.C) {
	p := &dockerProvisioner{storage: &cluster.MapStorage{}}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, p.storage, "")
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test-docker-client"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	client, err := p.GetDockerClient(nil)
	c.Assert(client, check.IsNil)
	c.Assert(err, check.ErrorMatches, "There is no Docker node. Add one with `tsuru node-add`")
}

func (s *S) TestDbAwareClientCreateContainer(c *check.C) {
	err := newFakeImage(s.p, "localhost:5000/myimg", nil)
	c.Assert(err, check.IsNil)
	client, err := s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/myimg",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	})
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	dbCont.MongoID = ""
	c.Assert(dbCont, check.DeepEquals, &container.Container{
		Container: types.Container{
			ID:       cont.ID,
			Name:     "mycont",
			AppName:  "myapp",
			Image:    "localhost:5000/myimg",
			HostAddr: "127.0.0.1",
			Status:   "building",
		},
	})
}

func (s *S) TestDbAwareClientCreateContainerNoAppNoName(c *check.C) {
	err := newFakeImage(s.p, "localhost:5000/myimg", nil)
	c.Assert(err, check.IsNil)
	client, err := s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: "localhost:5000/myimg",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(cont.ID)
	c.Assert(err, check.FitsTypeOf, &provision.UnitNotFoundError{})
	cont, err = client.CreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image:  "localhost:5000/myimg",
			Labels: map[string]string{},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(cont.ID)
	c.Assert(err, check.FitsTypeOf, &provision.UnitNotFoundError{})
}

func (s *S) TestDbAwareClientCreateContainerFailure(c *check.C) {
	err := newFakeImage(s.p, "localhost:5000/myimg", nil)
	c.Assert(err, check.IsNil)
	s.server.PrepareFailure("myerr", "/containers/create")
	client, err := s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	_, err = client.CreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/myimg",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	})
	c.Assert(err, check.ErrorMatches, `(?s).*myerr.*`)
	_, err = s.p.GetContainerByName("mycont")
	c.Assert(err, check.FitsTypeOf, &provision.UnitNotFoundError{})
}

func (s *S) TestDbAwareClientRemoveContainer(c *check.C) {
	err := newFakeImage(s.p, "localhost:5000/myimg", nil)
	c.Assert(err, check.IsNil)
	client, err := s.p.GetDockerClient(nil)
	c.Assert(err, check.IsNil)
	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: "mycont",
		Config: &docker.Config{
			Image: "localhost:5000/myimg",
			Labels: map[string]string{
				"app-name": "myapp",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = client.RemoveContainer(docker.RemoveContainerOptions{
		ID: cont.ID,
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(cont.ID)
	c.Assert(err, check.FitsTypeOf, &provision.UnitNotFoundError{})
}
