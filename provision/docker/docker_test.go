// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
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

func (s *S) newContainer(opts *newContainerOpts, p *dockerProvisioner) (*container, error) {
	container := container{
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
	routertest.FakeRouter.AddRoute(container.AppName, container.getAddress())
	port, err := getPort()
	if err != nil {
		return nil, err
	}
	ports := map[docker.Port]struct{}{
		docker.Port(port + "/tcp"): {},
	}
	config := docker.Config{
		Image:        image,
		Cmd:          []string{"ps"},
		ExposedPorts: ports,
	}
	_, c, err := p.getCluster().CreateContainer(docker.CreateContainerOptions{Config: &config})
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

func (s *S) removeTestContainer(c *container) error {
	routertest.FakeRouter.RemoveBackend(c.AppName)
	return c.remove(s.p)
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
	return p.getCluster().PullImage(opts, docker.AuthConfiguration{})
}

func (s *S) TestContainerGetAddress(c *check.C) {
	container := container{ID: "id123", HostAddr: "10.10.10.10", HostPort: "49153"}
	address := container.getAddress()
	expected := "http://10.10.10.10:49153"
	c.Assert(address.String(), check.Equals, expected)
}

func (s *S) TestContainerCreate(c *check.C) {
	config.Set("host", "my.cool.tsuru.addr:8080")
	defer config.Unset("host")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	app.SetEnv(bind.EnvVar{Name: "A", Value: "myenva"})
	app.SetEnv(bind.EnvVar{Name: "ABCD", Value: "other env"})
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	s.p.getCluster().PullImage(
		docker.PullImageOptions{Repository: "tsuru/brainfuck:latest"},
		docker.AuthConfiguration{},
	)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created", ProcessName: "myprocess1"}
	err := cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     s.p.getBuildImage(app),
		commands:    []string{"docker", "run"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	c.Assert(cont, check.FitsTypeOf, container{})
	c.Assert(cont.AppName, check.Equals, app.GetName())
	c.Assert(cont.Type, check.Equals, app.GetPlatform())
	u, _ := url.Parse(s.server.URL())
	host, _, _ := net.SplitHostPort(u.Host)
	c.Assert(cont.HostAddr, check.Equals, host)
	user, err := config.GetString("docker:user")
	c.Assert(err, check.IsNil)
	c.Assert(cont.User, check.Equals, user)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Path, check.Equals, "docker")
	c.Assert(container.Args, check.DeepEquals, []string{"run"})
	c.Assert(container.Config.User, check.Equals, user)
	c.Assert(container.Config.Memory, check.Equals, app.Memory)
	c.Assert(container.Config.MemorySwap, check.Equals, app.Memory+app.Swap)
	c.Assert(container.Config.CPUShares, check.Equals, int64(app.CpuShare))
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, []string{
		"A=myenva",
		"ABCD=other env",
		"TSURU_HOST=my.cool.tsuru.addr:8080",
		"TSURU_PROCESSNAME=myprocess1",
	})
}

func (s *S) TestContainerCreateSecurityOptions(c *check.C) {
	config.Set("docker:security-opts", []string{"label:type:svirt_apache", "ptrace peer=@unsecure"})
	defer config.Unset("docker:security-opts")
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	s.p.getCluster().PullImage(
		docker.PullImageOptions{Repository: "tsuru/brainfuck:latest"},
		docker.AuthConfiguration{},
	)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	err := cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     s.p.getBuildImage(app),
		commands:    []string{"docker", "run"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.SecurityOpts, check.DeepEquals, []string{"label:type:svirt_apache", "ptrace peer=@unsecure"})
}

func (s *S) TestContainerCreateAlocatesPort(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	s.p.getCluster().PullImage(
		docker.PullImageOptions{Repository: "tsuru/brainfuck:latest"},
		docker.AuthConfiguration{},
	)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	err := cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     s.p.getBuildImage(app),
		commands:    []string{"docker", "run"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	info, err := cont.networkInfo(s.p)
	c.Assert(err, check.IsNil)
	c.Assert(info.HTTPHostPort, check.Not(check.Equals), "")
}

func (s *S) TestContainerCreateDoesNotAlocatesPortForDeploy(c *check.C) {
	app := provisiontest.NewFakeApp("app-name", "brainfuck", 1)
	app.Memory = 15
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	s.p.getCluster().PullImage(
		docker.PullImageOptions{Repository: "tsuru/brainfuck:latest"},
		docker.AuthConfiguration{},
	)
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	err := cont.create(runContainerActionsArgs{
		isDeploy:    true,
		app:         app,
		imageID:     s.p.getBuildImage(app),
		commands:    []string{"docker", "run"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	info, err := cont.networkInfo(s.p)
	c.Assert(err, check.IsNil)
	c.Assert(info.HTTPHostPort, check.Equals, "")
}

func (s *S) TestContainerCreateUndefinedUser(c *check.C) {
	oldUser, _ := config.Get("docker:user")
	defer config.Set("docker:user", oldUser)
	config.Unset("docker:user")
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("app-name", "python", 1)
	routertest.FakeRouter.AddBackend(app.GetName())
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	cont := container{Name: "myName", AppName: app.GetName(), Type: app.GetPlatform(), Status: "created"}
	err = cont.create(runContainerActionsArgs{
		app:         app,
		imageID:     s.p.getBuildImage(app),
		commands:    []string{"docker", "run"},
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(&cont)
	dcli, _ := docker.NewClient(s.server.URL())
	container, err := dcli.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Config.User, check.Equals, "")
}

func (s *S) TestGetPort(c *check.C) {
	port, err := getPort()
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, s.port)
}

func (s *S) TestGetPortUndefined(c *check.C) {
	old, _ := config.Get("docker:run-cmd:port")
	defer config.Set("docker:run-cmd:port", old)
	config.Unset("docker:run-cmd:port")
	port, err := getPort()
	c.Assert(port, check.Equals, "")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetPortInteger(c *check.C) {
	old, _ := config.Get("docker:run-cmd:port")
	defer config.Set("docker:run-cmd:port", old)
	config.Set("docker:run-cmd:port", 8888)
	port, err := getPort()
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, "8888")
}

func (s *S) TestContainerSetStatus(c *check.C) {
	update := time.Date(1989, 2, 2, 14, 59, 32, 0, time.UTC).In(time.UTC)
	container := container{ID: "something-300", LastStatusUpdate: update}
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(container)
	defer coll.Remove(bson.M{"id": container.ID})
	container.setStatus(s.p, "what?!")
	c2, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, "what?!")
	lastUpdate := c2.LastStatusUpdate.In(time.UTC).Format(time.RFC822)
	c.Assert(lastUpdate, check.Not(check.DeepEquals), update.Format(time.RFC822))
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
}

func (s *S) TestContainerSetStatusStarted(c *check.C) {
	container := container{ID: "telnet"}
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(container)
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"id": container.ID})
	container.setStatus(s.p, provision.StatusStarted.String())
	c2, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
	c2.LastSuccessStatusUpdate = time.Time{}
	err = coll.Update(bson.M{"id": c2.ID}, c2)
	c.Assert(err, check.IsNil)
	c2.setStatus(s.p, provision.StatusStarting.String())
	c3, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(c3.LastSuccessStatusUpdate.IsZero(), check.Equals, false)
}

func (s *S) TestContainerSetStatusBuilding(c *check.C) {
	c1 := container{ID: "something-300", Status: provision.StatusBuilding.String()}
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(c1)
	defer coll.Remove(bson.M{"id": c1.ID})
	c1.setStatus(s.p, provision.StatusStarted.String())
	c2, err := s.p.getContainer(c1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Status, check.Equals, provision.StatusBuilding.String())
	c.Assert(c2.LastStatusUpdate.IsZero(), check.Equals, true)
	c.Assert(c2.LastSuccessStatusUpdate.IsZero(), check.Equals, true)
}

func (s *S) TestContainerSetImage(c *check.C) {
	container := container{ID: "something-300"}
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(container)
	defer coll.Remove(bson.M{"id": container.ID})
	container.setImage(s.p, "newimage")
	c2, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(c2.Image, check.Equals, "newimage")
}

func (s *S) TestContainerRemove(c *check.C) {
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(app.App{Name: "test-app"})
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": "test-app"})
	container, err := s.newContainer(&newContainerOpts{AppName: "test-app"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
	client, _ := docker.NewClient(s.server.URL())
	_, err = client.InspectContainer(container.ID)
	c.Assert(err, check.NotNil)
	_, ok := err.(*docker.NoSuchContainer)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestContainerRemoveIgnoreErrors(c *check.C) {
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(app.App{Name: "test-app"})
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": "test-app"})
	container, err := s.newContainer(&newContainerOpts{AppName: "test-app"}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	client, _ := docker.NewClient(s.server.URL())
	err = client.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID})
	c.Assert(err, check.IsNil)
	err = container.remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
}

func (s *S) TestContainerRemoveStopsContainer(c *check.C) {
	conn, err := db.Conn()
	defer conn.Close()
	a := app.App{Name: "test-app"}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	container, err := s.newContainer(&newContainerOpts{AppName: a.Name}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.start(s.p, &a, false)
	c.Assert(err, check.IsNil)
	err = container.remove(s.p)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": container.ID}).One(&container)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not found")
	client, _ := docker.NewClient(s.server.URL())
	_, err = client.InspectContainer(container.ID)
	c.Assert(err, check.NotNil)
	_, ok := err.(*docker.NoSuchContainer)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestContainerNetworkInfo(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	info, err := cont.networkInfo(s.p)
	c.Assert(err, check.IsNil)
	c.Assert(info.IP, check.Not(check.Equals), "")
	c.Assert(info.HTTPHostPort, check.Not(check.Equals), "")
}

func (s *S) TestContainerNetworkInfoNotFound(c *check.C) {
	inspectOut := `{
	"NetworkSettings": {
		"IpAddress": "10.10.10.10",
		"IpPrefixLen": 8,
		"Gateway": "10.65.41.1",
		"Ports": {}
	}
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/") {
			w.Write([]byte(inspectOut))
		}
	}))
	defer server.Close()
	var storage cluster.MapStorage
	storage.StoreContainer("c-01", server.URL)
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &storage,
		cluster.Node{Address: server.URL},
	)
	c.Assert(err, check.IsNil)
	container := container{ID: "c-01"}
	info, err := container.networkInfo(&p)
	c.Assert(err, check.IsNil)
	c.Assert(info.IP, check.Equals, "10.10.10.10")
	c.Assert(info.HTTPHostPort, check.Equals, "")
}

func (s *S) TestContainerShell(c *check.C) {
	var urls struct {
		items []url.URL
		sync.Mutex
	}
	s.server.SetHook(func(r *http.Request) {
		urls.Lock()
		urls.items = append(urls.items, *r.URL)
		urls.Unlock()
	})
	defer s.server.SetHook(nil)
	s.server.PrepareExec("*", func() {
		time.Sleep(500e6)
	})
	container, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewBufferString("")
	err = container.shell(s.p, stdin, &stdout, &stderr, pty{width: 140, height: 38, term: "xterm"})
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(stdout.String(), ""), check.Equals, true)
	urls.Lock()
	resizeURL := urls.items[len(urls.items)-2]
	urls.Unlock()
	matches := execResizeRegexp.FindStringSubmatch(resizeURL.Path)
	c.Assert(matches, check.HasLen, 2)
	c.Assert(resizeURL.Query().Get("w"), check.Equals, "140")
	c.Assert(resizeURL.Query().Get("h"), check.Equals, "38")
	client, _ := docker.NewClient(s.server.URL())
	exec, err := client.InspectExec(matches[1])
	c.Assert(err, check.IsNil)
	cmd := append([]string{exec.ProcessConfig.EntryPoint}, exec.ProcessConfig.Arguments...)
	c.Assert(cmd, check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "bash", "-l"})
}

func (s *S) TestGetContainer(c *check.C) {
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "abcdef", Type: "python"},
		container{ID: "fedajs", Type: "ruby"},
		container{ID: "wat", Type: "java"},
	)
	defer coll.RemoveAll(bson.M{"id": bson.M{"$in": []string{"abcdef", "fedajs", "wat"}}})
	container, err := s.p.getContainer("abcdef")
	c.Assert(err, check.IsNil)
	c.Assert(container.ID, check.Equals, "abcdef")
	c.Assert(container.Type, check.Equals, "python")
	container, err = s.p.getContainer("wut")
	c.Assert(container, check.IsNil)
	c.Assert(err, check.Equals, provision.ErrUnitNotFound)
}

func (s *S) TestGetContainers(c *check.C) {
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(
		container{ID: "abcdef", Type: "python", AppName: "something"},
		container{ID: "fedajs", Type: "python", AppName: "something"},
		container{ID: "wat", Type: "java", AppName: "otherthing"},
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app := &app.App{Name: "app1", Platform: "python", Deploys: 20}
	err = conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app.Name})
	cont := container{ID: "bleble", Type: app.Platform, AppName: app.Name, Image: "tsuru/app1"}
	coll := s.p.collection()
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

func (s *S) TestContainerCommit(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := bytes.Buffer{}
	nextImgName, err := appNewImageName(cont.AppName)
	c.Assert(err, check.IsNil)
	cont.BuildingImage = nextImgName
	imageId, err := cont.commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageId, check.Equals, repository)
}

func (s *S) TestContainerCommitWithRegistry(c *check.C) {
	config.Set("docker:registry-max-try", 1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := bytes.Buffer{}
	nextImgName, err := appNewImageName(cont.AppName)
	c.Assert(err, check.IsNil)
	cont.BuildingImage = nextImgName
	calls := 0
	s.server.SetHook(func(r *http.Request) {
		if ok, _ := regexp.MatchString("/images/.*?/push", r.URL.Path); ok {
			calls++
		}
	})
	defer s.server.SetHook(nil)
	imageId, err := cont.commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	repository := "localhost:3030/" + repoNamespace + "/app-" + cont.AppName + ":v1"
	c.Assert(imageId, check.Equals, repository)
	c.Assert(calls, check.Equals, 1)
}

func (s *S) TestContainerCommitErrorInPush(c *check.C) {
	s.server.PrepareFailure("push-failure", "/images/.*?/push")
	defer s.server.ResetFailure("push-failure")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := bytes.Buffer{}
	nextImgName, err := appNewImageName(cont.AppName)
	c.Assert(err, check.IsNil)
	cont.BuildingImage = nextImgName
	_, err = cont.commit(s.p, &buf)
	c.Assert(err, check.ErrorMatches, ".*push-failure\n")
}

func (s *S) TestContainerCommitRetryOnErrorInPush(c *check.C) {
	s.server.PrepareMultiFailures("i/o timeout", "/images/.*?/push")
	s.server.PrepareMultiFailures("i/o timeout", "/images/.*?/push")
	defer s.server.ResetMultiFailures()
	config.Unset("docker:registry-max-try")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := bytes.Buffer{}
	nextImgName, err := appNewImageName(cont.AppName)
	c.Assert(err, check.IsNil)
	cont.BuildingImage = nextImgName
	_, err = cont.commit(s.p, &buf)
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerCommitRetryShouldNotBeLessThanOne(c *check.C) {
	s.server.PrepareMultiFailures("i/o timeout", "/images/.*?/push")
	s.server.PrepareMultiFailures("i/o timeout", "/images/.*?/push")
	defer s.server.ResetMultiFailures()
	config.Set("docker:registry-max-try", -1)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := bytes.Buffer{}
	nextImgName, err := appNewImageName(cont.AppName)
	c.Assert(err, check.IsNil)
	cont.BuildingImage = nextImgName
	calls := 0
	s.server.SetHook(func(r *http.Request) {
		if ok, _ := regexp.MatchString("/images/.*?/push", r.URL.Path); ok {
			calls++
		}
	})
	defer s.server.SetHook(nil)
	_, err = cont.commit(s.p, &buf)
	c.Assert(err, check.IsNil)
	c.Assert(calls, check.Equals, 3)
}

func (s *S) TestGitDeploy(c *check.C) {
	stopCh := s.stopContainers(1)
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
	var conts []container
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(nil).All(&conts)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 0)
	err = s.p.getCluster().RemoveImage("tsuru/app-myapp:v1")
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
	var conts []container
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Find(nil).All(&conts)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 0)
	err = s.p.getCluster().RemoveImage("tsuru/myapp")
	c.Assert(err, check.NotNil)
}

func (s *S) TestArchiveDeploy(c *check.C) {
	stopCh := s.stopContainers(1)
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
	cont, err := s.p.start(&container{ProcessName: "web"}, app, imageId, &buf)
	c.Assert(err, check.IsNil)
	defer cont.remove(s.p)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.getContainer(cont.ID)
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
	defer cont.remove(s.p)
	c.Assert(cont.ID, check.Not(check.Equals), "")
	cont2, err := s.p.getContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont2.Image, check.Equals, imageId)
	c.Assert(cont2.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestContainerStop(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(cont.ID, nil)
	c.Assert(err, check.IsNil)
	err = cont.stop(s.p)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.getCluster().InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	c.Assert(cont.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestContainerStopReturnsNilWhenContainerAlreadyMarkedAsStopped(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	cont.setStatus(s.p, provision.StatusStopped.String())
	err = cont.stop(s.p)
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerLogs(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	var buff bytes.Buffer
	err = cont.logs(s.p, &buff)
	c.Assert(err, check.IsNil)
	c.Assert(buff.String(), check.Not(check.Equals), "")
}

func (s *S) TestUrlToHost(c *check.C) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"http://localhost:8081", "localhost"},
		{"http://localhost:3234", "localhost"},
		{"http://10.10.10.10:2375", "10.10.10.10"},
		{"", ""},
	}
	for _, t := range tests {
		c.Check(urlToHost(t.input), check.Equals, t.expected)
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
	clus := p.getCluster()
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
	err = p.pushImage("localhost:3030/base/img", "")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].URL.Path, check.Equals, "/images/create")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/base/img/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/base/img/push")
	c.Assert(requests[2].URL.RawQuery, check.Equals, "")
	err = s.newFakeImage(&p, "localhost:3030/base/img:v2", nil)
	c.Assert(err, check.IsNil)
	err = p.pushImage("localhost:3030/base/img", "v2")
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
	err = p.pushImage("localhost:3030/base/img", "")
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
	err = s.p.pushImage("localhost:3030/base", "")
	c.Assert(err, check.IsNil)
	c.Assert(request, check.IsNil)
}

func (s *S) TestContainerStart(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	contPath := fmt.Sprintf("/containers/%s/start", cont.ID)
	defer s.server.CustomHandler(contPath, s.server.DefaultHandler())
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 10
	err = cont.start(s.p, app, false)
	c.Assert(err, check.IsNil)
	dockerContainer, err = client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedLogOptions := map[string]string{
		"syslog-address": "udp://localhost:1514",
	}
	expectedPortBindings := map[docker.Port][]docker.PortBinding{
		"8888/tcp": {{HostIP: "", HostPort: ""}},
	}
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "always")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "syslog")
	c.Assert(dockerContainer.HostConfig.LogConfig.Config, check.DeepEquals, expectedLogOptions)
	c.Assert(dockerContainer.HostConfig.PortBindings, check.DeepEquals, expectedPortBindings)
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(10))
	c.Assert(cont.Status, check.Equals, "starting")
}

func (s *S) TestContainerStartDeployContainer(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	contPath := fmt.Sprintf("/containers/%s/start", cont.ID)
	defer s.server.CustomHandler(contPath, s.server.DefaultHandler())
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	app.Memory = 15
	app.Swap = 15
	app.CpuShare = 50
	err = cont.start(s.p, app, true)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Status, check.Equals, "building")
	dockerContainer, err := client.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.HostConfig.RestartPolicy.Name, check.Equals, "")
	c.Assert(dockerContainer.HostConfig.LogConfig.Type, check.Equals, "")
	c.Assert(dockerContainer.HostConfig.Memory, check.Equals, int64(15))
	c.Assert(dockerContainer.HostConfig.MemorySwap, check.Equals, int64(30))
	c.Assert(dockerContainer.HostConfig.CPUShares, check.Equals, int64(50))
}

func (s *S) TestContainerStartWithoutPort(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	oldUser, _ := config.Get("docker:run-cmd:port")
	defer config.Set("docker:run-cmd:port", oldUser)
	config.Unset("docker:run-cmd:port")
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	err = cont.start(s.p, app, false)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerStartStartedUnits(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	err = cont.start(s.p, app, false)
	c.Assert(err, check.IsNil)
	err = cont.start(s.p, app, false)
	c.Assert(err, check.NotNil)
}

func (s *S) TestContainerAvailable(c *check.C) {
	cases := map[provision.Status]bool{
		provision.StatusCreated:  false,
		provision.StatusStarting: true,
		provision.StatusStarted:  true,
		provision.StatusError:    false,
		provision.StatusStopped:  false,
		provision.StatusBuilding: false,
	}
	for status, expected := range cases {
		cont := container{Status: status.String()}
		c.Assert(cont.available(), check.Equals, expected)
	}
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

func (s *S) TestContainerExec(c *check.C) {
	container, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	err = container.exec(s.p, &stdout, &stderr, "ls", "-lh")
	c.Assert(err, check.IsNil)
}

func (s *S) TestContainerExecErrorCode(c *check.C) {
	s.server.CustomHandler("/exec/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ID":"id","ExitCode":9}`))
	}))
	container, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	var stdout, stderr bytes.Buffer
	err = container.exec(s.p, &stdout, &stderr, "ls", "-lh")
	c.Assert(err, check.DeepEquals, &execErr{code: 9})
}
