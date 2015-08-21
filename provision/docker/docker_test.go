// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var execResizeRegexp = regexp.MustCompile(`^.*/exec/(.*)/resize$`)

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
		ID:          "id",
		IP:          "10.10.10.10",
		HostPort:    "3333",
		HostAddr:    "127.0.0.1",
		ProcessName: "web",
	}
	if p == nil {
		p = s.p
	}
	image := "tsuru/python:latest"
	var customData map[string]interface{}
	if opts != nil {
		if opts.Image != "" {
			image = opts.Image
		}
		container.Status = opts.Status
		container.AppName = opts.AppName
		container.ProcessName = opts.ProcessName
		customData = opts.ImageCustomData
		if opts.Provisioner != nil {
			p = opts.Provisioner
		}
	}
	err := s.newFakeImage(p, image, customData)
	if err != nil {
		return nil, err
	}
	if container.AppName == "" {
		container.AppName = "container"
	}
	routertest.FakeRouter.AddBackend(container.AppName)
	routertest.FakeRouter.AddRoute(container.AppName, container.Address())
	ports := map[docker.Port]struct{}{
		docker.Port(s.port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        image,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	_, c, err := p.Cluster().CreateContainer(docker.CreateContainerOptions{Config: &config})
	if err != nil {
		return nil, err
	}
	container.ID = c.ID
	container.Image = image
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Collection(s.collName).Insert(&container)
	if err != nil {
		return nil, err
	}
	imageId, err := appCurrentImageName(container.AppName)
	if err != nil {
		return nil, err
	}
	err = s.newFakeImage(p, imageId, nil)
	if err != nil {
		return nil, err
	}
	return &container, nil
}

func (s *S) removeTestContainer(c *container.Container) error {
	routertest.FakeRouter.RemoveBackend(c.AppName)
	return c.Remove(s.p)
}

func (s *S) newFakeImage(p *dockerProvisioner, repo string, customData map[string]interface{}) error {
	if customData == nil {
		customData = map[string]interface{}{
			"procfile": "web: python myapp.py",
		}
	}
	var buf safe.Buffer
	opts := docker.PullImageOptions{Repository: repo, OutputStream: &buf}
	err := saveImageCustomData(repo, customData)
	if err != nil && !mgo.IsDup(err) {
		return err
	}
	return p.Cluster().PullImage(opts, docker.AuthConfiguration{})
}

func (s *S) TestGetContainer(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{ID: "abcdef", Type: "python"},
		container.Container{ID: "fedajs", Type: "ruby"},
		container.Container{ID: "wat", Type: "java"},
	)
	defer coll.RemoveAll(bson.M{"id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	container, err := s.p.GetContainer("abcdef")
	c.Assert(err, check.IsNil)
	c.Assert(container.ID, check.Equals, "abcdef")
	c.Assert(container.Type, check.Equals, "python")
	container, err = s.p.GetContainer("wut")
	c.Assert(container, check.IsNil)
	c.Assert(err, check.Equals, provision.ErrUnitNotFound)
}

func (s *S) TestGetContainers(c *check.C) {
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(
		container.Container{ID: "abcdef", Type: "python", AppName: "something"},
		container.Container{ID: "fedajs", Type: "python", AppName: "something"},
		container.Container{ID: "wat", Type: "java", AppName: "otherthing"},
	)
	defer coll.RemoveAll(bson.M{"id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
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
	img := s.p.getBuildImage(app)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/python:latest", repoNamespace))
}

func (s *S) TestGetImageAppWhenDeployIsMultipleOf10(c *check.C) {
	app := &app.App{Name: "app1", Platform: "python", Deploys: 20}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	cont := container.Container{ID: "bleble", Type: app.Platform, AppName: app.Name, Image: "tsuru/app1"}
	coll := s.p.Collection()
	err = coll.Insert(cont)
	c.Assert(err, check.IsNil)
	defer coll.Close()
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"id": cont.ID})
	img := s.p.getBuildImage(app)
	repoNamespace, err := config.GetString("docker:repository-namespace")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, fmt.Sprintf("%s/%s:latest", repoNamespace, app.Platform))
}

func (s *S) TestGetImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	img := s.p.getBuildImage(app)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	expected := fmt.Sprintf("localhost:3030/%s/python:latest", repoNamespace)
	c.Assert(img, check.Equals, expected)
}

func (s *S) TestGitDeploy(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	repository.Manager().CreateRepository("myapp", nil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	imageId, err := s.p.gitDeploy(app, "ff13e", &buf)
	c.Assert(err, check.IsNil)
	c.Assert(imageId, check.Equals, "tsuru/app-myapp:v1")
	var conts []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(nil).All(&conts)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 0)
	err = s.p.Cluster().RemoveImage("tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
}

type errBuffer struct{}

func (errBuffer) Write(data []byte) (int, error) {
	return 0, fmt.Errorf("My write error")
}

func (s *S) TestGitDeployRollsbackAfterErrorOnAttach(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	repository.Manager().CreateRepository("myapp", nil)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	var buf errBuffer
	_, err = s.p.gitDeploy(app, "ff13e", &buf)
	c.Assert(err, check.ErrorMatches, `.*My write error`)
	var conts []container.Container
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(nil).All(&conts)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 0)
	err = s.p.Cluster().RemoveImage("tsuru/myapp")
	c.Assert(err, check.NotNil)
}

func (s *S) TestArchiveDeploy(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	_, err = s.p.archiveDeploy(app, s.p.getBuildImage(app), "https://s3.amazonaws.com/wat/archive.tar.gz", &buf)
	c.Assert(err, check.IsNil)
}

func (s *S) TestStart(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageId := s.p.getBuildImage(app)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	cont, err := s.p.start(&container.Container{ProcessName: "web"}, app, imageId, &buf)
	c.Assert(err, check.IsNil)
	defer cont.Remove(s.p)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, imageId)
	c.Assert(cont2.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestStartStoppedContainer(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.Status = provision.StatusStopped.String()
	err = s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	imageId := s.p.getBuildImage(app)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	var buf bytes.Buffer
	cont, err = s.p.start(cont, app, imageId, &buf)
	c.Assert(err, check.IsNil)
	defer cont.Remove(s.p)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.GetContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, imageId)
	c.Assert(cont2.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestStartTsuruAllocatorStress(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	config.Set("docker:port-allocator", "tsuru")
	defer config.Unset("docker:port-allocator")
	alocPorts := map[string]struct{}{}
	var mut sync.Mutex
	s.server.CustomHandler("/containers/.*/start", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)
		c.Assert(err, check.IsNil)
		var conf docker.HostConfig
		err = json.Unmarshal(data, &conf)
		c.Assert(err, check.IsNil)
		port := conf.PortBindings["8888/tcp"][0].HostPort
		mut.Lock()
		if _, present := alocPorts[port]; present {
			mut.Unlock()
			http.Error(w, "port already allocated", http.StatusInternalServerError)
			return
		}
		alocPorts[port] = struct{}{}
		mut.Unlock()
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	err := s.newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	imageId, err := appCurrentImageName(app.GetName())
	wg := sync.WaitGroup{}
	conts := make([]*container.Container, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cont, err := s.p.start(&container.Container{ProcessName: "web"}, app, imageId, ioutil.Discard)
			c.Assert(err, check.IsNil)
			conts[i] = cont
		}(i)
	}
	wg.Wait()
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	c.Assert(alocPorts, check.HasLen, len(conts))
	for _, cont := range conts {
		dockerContainer, err := client.InspectContainer(cont.ID)
		c.Assert(err, check.IsNil)
		c.Assert(dockerContainer.State.Running, check.Equals, true)
		port := dockerContainer.HostConfig.PortBindings["8888/tcp"][0].HostPort
		c.Assert(port, check.Not(check.Equals), "")
	}
}

type NodeList []cluster.Node

func (a NodeList) Len() int           { return len(a) }
func (a NodeList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a NodeList) Less(i, j int) bool { return a[i].Address < a[j].Address }

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
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(&p, "localhost:3030/base/img", nil)
	c.Assert(err, check.IsNil)
	err = p.PushImage("localhost:3030/base/img", "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[2].URL.RawQuery, check.Equals, "")
	err = s.newFakeImage(&p, "localhost:3030/base/img:v2", nil)
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
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(&p, "localhost:3030/base/img", nil)
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
