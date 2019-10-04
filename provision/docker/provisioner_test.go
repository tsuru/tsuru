// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	internalNodeContainer "github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

func newFakeServer() *httptest.Server {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake image")
	})
	return httptest.NewServer(fakeHandler)
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	p, err := provision.Get("docker")
	c.Assert(err, check.IsNil)
	c.Assert(p, check.FitsTypeOf, &dockerProvisioner{})
}

func (s *S) TestProvisionerProvision(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	err := s.p.Provision(app)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, true)
}

func (s *S) TestProvisionerRestart(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 1)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "web",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "worker",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = s.p.Start(app, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.Cluster().InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	dockerContainer, err = s.p.Cluster().InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Restart(app, "", nil)
	c.Assert(err, check.IsNil)
	dbConts, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 2)
	c.Assert(dbConts[0].ID, check.Not(check.Equals), cont1.ID)
	c.Assert(dbConts[0].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[0].Status, check.Equals, provision.StatusStarting.String())
	c.Assert(dbConts[1].ID, check.Not(check.Equals), cont2.ID)
	c.Assert(dbConts[1].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[1].Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = s.p.Cluster().InspectContainer(dbConts[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(dbConts[0].IP, check.Equals, expectedIP)
	c.Assert(dbConts[0].HostPort, check.Equals, expectedPort)
}

func (s *S) TestProvisionerRestartStoppedContainer(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 1)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "web",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "worker",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = s.p.Stop(app, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.Cluster().InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = s.p.Cluster().InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	err = s.p.Restart(app, "", nil)
	c.Assert(err, check.IsNil)
	dbConts, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 2)
	c.Assert(dbConts[0].ID, check.Not(check.Equals), cont1.ID)
	c.Assert(dbConts[0].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[0].Status, check.Equals, provision.StatusStarting.String())
	c.Assert(dbConts[1].ID, check.Not(check.Equals), cont1.ID)
	c.Assert(dbConts[1].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[1].Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = s.p.Cluster().InspectContainer(dbConts[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(dbConts[0].IP, check.Equals, expectedIP)
	c.Assert(dbConts[0].HostPort, check.Equals, expectedPort)
}

type containerByProcessList []container.Container

func (l containerByProcessList) Len() int           { return len(l) }
func (l containerByProcessList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l containerByProcessList) Less(i, j int) bool { return l[i].ProcessName < l[j].ProcessName }

func (s *S) TestProvisionerRestartProcess(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 1)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "web",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         app.GetName(),
		ProcessName:     "worker",
		ImageCustomData: customData,
		Image:           "tsuru/app-" + app.GetName(),
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = s.p.Start(app, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.Cluster().InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	dockerContainer, err = s.p.Cluster().InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Restart(app, "web", nil)
	c.Assert(err, check.IsNil)
	dbConts, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 2)
	sort.Sort(containerByProcessList(dbConts))
	c.Assert(dbConts[1].ID, check.Equals, cont2.ID)
	c.Assert(dbConts[0].ID, check.Not(check.Equals), cont1.ID)
	c.Assert(dbConts[0].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[0].Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = s.p.Cluster().InspectContainer(dbConts[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(dbConts[0].IP, check.Equals, expectedIP)
	c.Assert(dbConts[0].HostPort, check.Equals, expectedPort)
}

func (s *S) stopContainers(endpoint string, n uint) <-chan bool {
	ch := make(chan bool)
	go func() {
		defer close(ch)
		client, err := docker.NewClient(endpoint)
		if err != nil {
			return
		}
		for n > 0 {
			opts := docker.ListContainersOptions{All: false}
			containers, err := client.ListContainers(opts)
			if err != nil {
				return
			}
			if len(containers) > 0 {
				for _, cont := range containers {
					if cont.ID != "" {
						client.StopContainer(cont.ID, 1)
						n--
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	return ch
}

func (s *S) TestDeploy(c *check.C) {
	config.Unset("docker:repository-namespace")
	defer config.Set("docker:repository-namespace", "tsuru")
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := s.newApp("myapp")
	err = app.CreateApp(&a, s.user)
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, "myapp")
		c.Assert(inUse, check.Equals, 1)
		a.Quota.InUse = 1
		return nil
	}
	c.Assert(err, check.IsNil)
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, nil, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	builderImgID := s.team.Name + "/app-" + a.Name + ":v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: s.team.Name + "/app-" + a.Name,
		Tag:        "v1-builder",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	imgID, err := s.p.Deploy(&a, builderImgID, evt)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-"+a.Name+":v1")
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].IP)
	c.Assert(a.Quota, check.DeepEquals, quota.Quota{Limit: -1, InUse: 1})
	cont, err := s.p.Cluster().InspectContainer(units[0].GetID())
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Cmd, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
}

func (s *S) TestDeployWithLimiterActive(c *check.C) {
	config.Set("docker:limit:actions-per-host", 1)
	defer config.Unset("docker:limit:actions-per-host")
	var p dockerProvisioner
	p.storage = &cluster.MapStorage{}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	p.cluster, err = cluster.New(nil, p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "test-default"}},
	)
	c.Assert(err, check.IsNil)
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := s.newApp("otherapp")
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	fakeServer := newFakeServer()
	defer fakeServer.Close()
	builderImgID := "tsuru/app-" + a.Name + ":v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-" + a.Name,
		Tag:        "v1-builder",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(&a, builderImgID, evt)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	hostAddr := net.URLToHost(s.server.URL())
	c.Assert(p.ActionLimiter().Len(hostAddr), check.Equals, 0)
	err = p.Destroy(&a)
	c.Assert(err, check.IsNil)
	c.Assert(p.ActionLimiter().Len(hostAddr), check.Equals, 0)
}

func (s *S) TestDeployWithLimiterGlobalActive(c *check.C) {
	config.Set("docker:limit:mode", "global")
	config.Set("docker:limit:actions-per-host", 1)
	defer config.Unset("docker:limit:mode")
	defer config.Unset("docker:limit:actions-per-host")
	var p dockerProvisioner
	p.storage = &cluster.MapStorage{}
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	p.cluster, err = cluster.New(nil, p.storage, "",
		cluster.Node{Address: s.server.URL(), Metadata: map[string]string{"pool": "test-default"}},
	)
	c.Assert(err, check.IsNil)
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := s.newApp("otherapp")
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	fakeServer := newFakeServer()
	defer fakeServer.Close()
	builderImgID := "tsuru/app-" + a.Name + ":v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-" + a.Name,
		Tag:        "v1-builder",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	imgID, err := s.p.Deploy(&a, builderImgID, evt)
	c.Assert(err, check.IsNil)
	c.Assert(imgID, check.Equals, "tsuru/app-"+a.Name+":v1")
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	hostAddr := net.URLToHost(s.server.URL())
	c.Assert(p.ActionLimiter().Len(hostAddr), check.Equals, 0)
	err = p.Destroy(&a)
	c.Assert(err, check.IsNil)
	c.Assert(p.ActionLimiter().Len(hostAddr), check.Equals, 0)
}

func (s *S) TestDeployQuotaExceeded(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := s.newApp("otherapp")
	err = app.CreateApp(&a, s.user)
	s.mockService.AppQuota.OnSetLimit = func(appName string, limit int) error {
		c.Assert(appName, check.Equals, "otherapp")
		c.Assert(limit, check.Equals, 1)
		a.Quota.Limit = 1
		return nil
	}
	s.mockService.AppQuota.OnSet = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, "otherapp")
		c.Assert(quantity, check.Equals, 2)
		return &quota.QuotaExceededError{Available: 1, Requested: 2}
	}
	c.Assert(err, check.IsNil)
	err = a.SetQuotaLimit(1)
	c.Assert(err, check.IsNil)
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, nil, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python myworker.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	fakeServer := newFakeServer()
	defer fakeServer.Close()
	builderImgID := "tsuru/app-" + a.Name + ":v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-" + a.Name,
		Tag:        "v1-builder",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(&a, builderImgID, evt)
	c.Assert(err, check.NotNil)
	compErr, ok := err.(*errors.CompositeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(compErr.Message, check.Equals, "Cannot start application units")
	e, ok := compErr.Base.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestDeployCanceledEvent(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend(app)
	defer routertest.FakeRouter.RemoveBackend(app.GetName())
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: "app", Value: "myapp"},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Cancelable:    true,
		Allowed:       event.Allowed(permission.PermApp),
		AllowedCancel: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	fakeServer := newFakeServer()
	defer fakeServer.Close()
	builderImgID := "tsuru/app-" + app.GetName() + ":v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-" + app.GetName(),
		Tag:        "v1-builder",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	done := make(chan bool)
	go func() {
		defer close(done)
		img, depErr := s.p.Deploy(app, builderImgID, evt)
		c.Assert(depErr, check.ErrorMatches, "unit creation canceled by user action")
		c.Assert(img, check.Equals, "")
	}()
	time.Sleep(100 * time.Millisecond)
	evtDB, err := event.GetByID(evt.UniqueID)
	c.Assert(err, check.IsNil)
	err = evtDB.TryCancel("because yes", "majortom@ground.control")
	c.Assert(err, check.IsNil)
	<-done
}

func (s *S) TestDeployRegisterRace(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	var p dockerProvisioner
	var registerCount int64
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		go func(path string) {
			parts := strings.Split(path, "/")
			if len(parts) == 4 && parts[3] == "start" {
				registerErr := p.RegisterUnit(nil, parts[2], nil)
				if registerErr == nil {
					atomic.AddInt64(&registerCount, 1)
				} else {
					c.Fatal(registerErr)
				}
			}
		}(r.URL.Path)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL()})
	c.Assert(err, check.IsNil)
	err = newFakeImage(&p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	nTests := 100
	stopCh := s.stopContainers(server.URL(), uint(nTests))
	defer func() { <-stopCh }()
	wg := sync.WaitGroup{}
	for i := 0; i < nTests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("myapp-%d", i)
			app := provisiontest.NewFakeApp(name, "python", 1)
			routertest.FakeRouter.AddBackend(app)
			defer routertest.FakeRouter.RemoveBackend(app.GetName())
			baseImage, err := image.GetBuildImage(app)
			c.Assert(err, check.IsNil)
			img, err := p.deployPipeline(app, baseImage, []string{"/bin/test"}, nil)
			c.Assert(err, check.IsNil)
			c.Assert(img, check.Equals, "localhost:3030/tsuru/app-"+name+":v1")
		}(i)
	}
	wg.Wait()
	c.Assert(registerCount, check.Equals, int64(nTests))
}
func (s *S) TestRollbackDeploy(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-otherapp:v1", nil)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := s.newApp("otherapp")
	a.Quota = quota.UnlimitedQuota
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	w := safe.NewBuffer(make([]byte, 2048))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestDeployErasesOldImagesIfFailed(c *check.C) {
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := s.newApp("appdeployimagetest")
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	baseImgName := "tsuru/app-" + a.Name + ":v1"
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData(baseImgName, customData)
	c.Assert(err, check.IsNil)
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var result docker.Config
		jsonErr := json.Unmarshal(data, &result)
		if jsonErr == nil {
			if result.Image == baseImgName {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("my awesome error"))
				return
			}
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(&a, baseImgName+"-builder", evt)
	c.Assert(err, check.ErrorMatches, ".*my awesome error.*")
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	expected := []string{"tsuru/app-appdeployimagetest:v1-builder", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestRollbackDeployFailureDoesntEraseImage(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-otherapp:v1", nil)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := s.newApp("otherapp")
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	s.p.Provision(&a)
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var result docker.Config
		jsonErr := json.Unmarshal(data, &result)
		if jsonErr == nil {
			if result.Image == "tsuru/app-otherapp:v1" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	w := safe.NewBuffer(make([]byte, 2048))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	_, err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
		Rollback:     true,
		Event:        evt,
	})
	c.Assert(err, check.NotNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/app-otherapp:v1", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestDeployImageID(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	a := s.newApp("myapp")
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, nil, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": []string{"/bin/sh", "-c", "python test.py"},
		},
	}
	builderImgID := "tsuru/app-" + a.Name + ":v1"
	err = image.SaveImageCustomData(builderImgID, customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: "app", Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-" + a.Name,
		Tag:        "v1",
	}
	err = s.p.Cluster().PullImage(pullOpts, dockercommon.RegistryAuthConfig(pullOpts.Repository))
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(&a, builderImgID, evt)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	appCurrentImage, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	imd, err := image.GetImageMetaData(appCurrentImage)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string][]string{"web": {"/bin/sh", "-c", "python test.py"}}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(units[0].GetID())
	c.Assert(err, check.IsNil)
	expectedPortBindings := map[docker.Port][]docker.PortBinding{
		"8888/tcp": {{HostIP: "", HostPort: ""}},
	}
	c.Assert(dockerContainer.HostConfig.PortBindings, check.DeepEquals, expectedPortBindings)
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	unit := cont.AsUnit(a)
	a.BindUnit(&unit)
	s.p.Provision(a)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	count, err := coll.Find(bson.M{"appname": cont.AppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, false)
	c.Assert(a.HasBind(&unit), check.Equals, false)
}

func (s *S) TestProvisionerDestroyEmptyUnit(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	err := s.p.Destroy(a)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerAddUnits(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	_, err = s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 4)
	coll := s.p.Collection()
	defer coll.Close()
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 4)
}

func (s *S) TestProvisionerAddUnitsInvalidProcess(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	_, err = s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "bogus", nil)
	c.Assert(err, check.FitsTypeOf, provision.InvalidProcessError{})
	c.Assert(err, check.ErrorMatches, `process error: no command declared in Procfile for process "bogus"`)
}

func (s *S) TestProvisionerAddUnitsWithErrorDoesntLeaveLostUnits(c *check.C) {
	callCount := 0
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer s.server.CustomHandler("/containers/create", s.server.DefaultHandler())
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{Container: types.Container{ID: "c-89320", AppName: a.GetName(), Version: "a345fe", Image: "tsuru/python:latest"}})
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.NotNil)
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestProvisionerAddZeroUnits(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{Container: types.Container{ID: "c-89320", AppName: a.GetName(), Version: "a345fe", Image: "tsuru/python:latest"}})
	err = s.p.AddUnits(a, 0, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units")
}

func (s *S) TestProvisionerAddUnitsWithNoDeploys(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	s.p.Provision(a)
	err := s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "New units can only be added after the first deployment")
}

func (s *S) TestProvisionerAddUnitsWithHost(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(a)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{Container: types.Container{ID: "xxxfoo", AppName: a.GetName(), Version: "123987", Image: "tsuru/python:latest"}})
	imageID, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         a,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].HostAddr, check.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 2)
}

func (s *S) TestProvisionerAddUnitsWithHostPartialRollback(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	imageID, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	var callCount int32
	s.server.CustomHandler("/containers/.*/start", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&callCount, 1) == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         a,
		imageID:     imageID,
		provisioner: s.p,
	})
	c.Assert(err, check.ErrorMatches, "(?s).*error in docker node.*")
	c.Assert(units, check.HasLen, 0)
	coll := s.p.Collection()
	defer coll.Close()
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestProvisionerRemoveUnits(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "1"}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "worker", HostAddr: "url0", HostPort: "2"}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "3"}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage, "")
	c.Assert(err, check.IsNil)
	s.p.cluster = clusterInstance
	s.p.scheduler = &scheduler
	err = clusterInstance.Register(cluster.Node{
		Address:  "http://url0:1234",
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a1.Name, customData)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	conts := []container.Container{cont1, cont2, cont3}
	units := []provision.Unit{cont1.AsUnit(papp), cont2.AsUnit(papp), cont3.AsUnit(papp)}
	for i := range conts {
		err = routertest.FakeRouter.AddRoutes(a1.Name, []*url.URL{conts[i].Address()})
		c.Assert(err, check.IsNil)
		err = papp.BindUnit(&units[i])
		c.Assert(err, check.IsNil)
	}
	err = s.p.RemoveUnits(papp, 2, "web", nil)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(conts[0].ID)
	c.Assert(err, check.NotNil)
	_, err = s.p.GetContainer(conts[1].ID)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(conts[2].ID)
	c.Assert(err, check.NotNil)
	c.Assert(s.p.scheduler.ignoredContainers, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[0].Address().String()), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[1].Address().String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[2].Address().String()), check.Equals, false)
	c.Assert(papp.HasBind(&units[0]), check.Equals, false)
	c.Assert(papp.HasBind(&units[1]), check.Equals, true)
	c.Assert(papp.HasBind(&units[2]), check.Equals, false)
}

func (s *S) TestProvisionerRemoveUnitsFailRemoveOldRoute(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "1"}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "worker", HostAddr: "url0", HostPort: "2"}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "3"}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage, "")
	c.Assert(err, check.IsNil)
	s.p.cluster = clusterInstance
	s.p.scheduler = &scheduler
	err = clusterInstance.Register(cluster.Node{
		Address:  "http://url0:1234",
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a1.Name, customData)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	conts := []container.Container{cont1, cont2, cont3}
	units := []provision.Unit{cont1.AsUnit(papp), cont2.AsUnit(papp), cont3.AsUnit(papp)}
	for i := range conts {
		err = routertest.FakeRouter.AddRoutes(a1.Name, []*url.URL{conts[i].Address()})
		c.Assert(err, check.IsNil)
		err = papp.BindUnit(&units[i])
		c.Assert(err, check.IsNil)
	}
	routertest.FakeRouter.FailForIp(conts[2].Address().String())
	err = s.p.RemoveUnits(papp, 2, "web", nil)
	c.Assert(err, check.ErrorMatches, "error removing routes, units weren't removed: Forced failure")
	_, err = s.p.GetContainer(conts[0].ID)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(conts[1].ID)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(conts[2].ID)
	c.Assert(err, check.IsNil)
	c.Assert(s.p.scheduler.ignoredContainers, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[0].Address().String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[1].Address().String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, conts[2].Address().String()), check.Equals, true)
	c.Assert(papp.HasBind(&units[0]), check.Equals, true)
	c.Assert(papp.HasBind(&units[1]), check.Equals, true)
	c.Assert(papp.HasBind(&units[2]), check.Equals, true)
}

func (s *S) TestProvisionerRemoveUnitsEmptyProcess(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage, "")
	c.Assert(err, check.IsNil)
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	err = clusterInstance.Register(cluster.Node{
		Address:  s.server.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnits(papp, 1, "", nil)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetContainer(cont1.ID)
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerRemoveUnitsNotFound(c *check.C) {
	err := s.p.RemoveUnits(nil, 1, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: app should not be nil")
}

func (s *S) TestProvisionerRemoveUnitsZeroUnits(c *check.C) {
	err := s.p.RemoveUnits(provisiontest.NewFakeApp("something", "python", 0), 0, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cannot remove zero units")
}

func (s *S) TestProvisionerRemoveUnitsTooManyUnits(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web"}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "web"}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web"}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage, "")
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	err = clusterInstance.Register(cluster.Node{
		Address:  "http://url0:1234",
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a1.Name, customData)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnits(papp, 4, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cannot remove 4 units from process \"web\", only 3 available")
}

func (s *S) TestProvisionerRemoveUnitsInvalidProcess(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage, "")
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	err = clusterInstance.Register(cluster.Node{
		Address:  s.server.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-"+a1.Name, customData)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnits(papp, 1, "worker", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `process error: no command declared in Procfile for process "worker"`)
}

func (s *S) TestProvisionerSetUnitStatus(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusError)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
	c.Assert(container.ExpectedStatus(), check.Equals, provision.StatusStarted)
}

func (s *S) TestProvisionerSetUnitStatusAsleep(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.Sleep(s.p.ClusterClient(), s.p.ActionLimiter())
	c.Assert(err, check.IsNil)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStopped)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusAsleep.String())
}

func (s *S) TestProvisionerSetUnitStatusUpdatesIp(c *check.C) {
	err := s.conn.Apps().Insert(&app.App{Name: "myawesomeapp"})
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStarted)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
	c.Assert(container.IP, check.Matches, `\d+.\d+.\d+.\d+`)
}

func (s *S) TestProvisionerSetUnitStatusWrongApp(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName + "a"}, provision.StatusError)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "wrong app name")
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestProvisionSetUnitStatusNoAppName(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID}, provision.StatusError)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusUnitNotFound(c *check.C) {
	err := s.p.SetUnitStatus(provision.Unit{ID: "mycontainer", AppName: "myapp"}, provision.StatusError)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "mycontainer")
}

func (s *S) TestProvisionerSetUnitStatusBuildingContainer(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID}, provision.StatusStarted)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusBuilding.String())
}

func (s *S) TestProvisionerSetUnitStatusSearchByName(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: "invalid-id", Name: container.Name, AppName: container.AppName}, provision.StatusError)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusUnexpectedStopped(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStopped)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusExpectedStopped(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStopped.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStopped)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStopped.String())
}

func (s *S) TestProvisionerSetUnitStatusUnexpectedStarted(c *check.C) {
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStopped.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStarted)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
	c.Assert(container.ExpectedStatus(), check.Equals, provision.StatusStopped)
}

func (s *S) TestProvisionerExecuteCommand(c *check.C) {
	a := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container1)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container1.ID}, container1)
	container2, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	coll.Update(bson.M{"id": container2.ID}, container2)
	var executed bool
	s.server.PrepareExec("*", func() {
		executed = true
	})
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Units:  []string{container1.ID, container2.ID},
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, true)
}
func (s *S) TestProvisionerExecuteCommandSingleContainer(c *check.C) {
	a := provisiontest.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	var stdout, stderr bytes.Buffer
	var executed bool
	s.server.PrepareExec("*", func() {
		executed = true
	})
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Units:  []string{container.ID},
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandNoUnits(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-almah", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	a.SetEnv(bind.EnvVar{Name: "ENV", Value: "OK"})
	var stdout, stderr bytes.Buffer
	var created bool
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		created = true
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var config docker.Config
		json.Unmarshal(data, &config)
		sort.Strings(config.Env)
		c.Assert(config.Env, check.DeepEquals, []string{"ENV=OK", "PORT=8888", "TSURU_HOST=", "TSURU_PROCESSNAME=", "port=8888"})
		var createOpts docker.CreateContainerOptions
		json.Unmarshal(data, &createOpts)
		c.Assert(createOpts.HostConfig, check.NotNil)
		c.Assert(createOpts.HostConfig.AutoRemove, check.Equals, true)
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	s.server.CustomHandler("/containers/.*/attach", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot hijack connection", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		conn, _, cErr := hijacker.Hijack()
		if cErr != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outStream := stdcopy.NewStdWriter(conn, stdcopy.Stdout)
		fmt.Fprintf(outStream, "test")
		errStream := stdcopy.NewStdWriter(conn, stdcopy.Stderr)
		fmt.Fprintf(errStream, "errtest")
		conn.Close()
	}))
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "test")
	c.Assert(stderr.String(), check.Equals, "errtest")
	c.Assert(created, check.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandNoUnitsNoImage(c *check.C) {
	s.server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no actual pull executed
		w.WriteHeader(http.StatusOK)
	}))
	a := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &buf,
		Stderr: &buf,
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.ErrorMatches, ".*no such image.*")
}

func (s *S) TestProvisionerCollection(c *check.C) {
	collection := s.p.Collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, s.collName)
}

func (s *S) TestProvisionerCollectionDefaultConfig(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Unset("docker:collection")
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	col := p.Collection()
	defer col.Close()
	c.Assert(col.Name, check.Equals, "dockercluster")
	config.Set("docker:collection", s.collName)
}

func (s *S) TestProvisionerCollectionErrorConfig(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:collection", true)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.ErrorMatches, ".*value for the key.*is not a string.*")
	config.Set("docker:collection", s.collName)
}

func (s *S) TestProvisionerRollbackNoDeployImage(c *check.C) {
	a := provisiontest.NewFakeApp("otherapp", "python", 1)
	_, err := s.p.Rollback(a, "inexist", nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*image.ImageNotFoundErr)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.App, check.Equals, "otherapp")
	c.Assert(e.Image, check.Equals, "inexist")
}

func (s *S) TestProvisionerStart(c *check.C) {
	err := s.conn.Apps().Insert(&app.App{Name: "almah"})
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	cont1, err = s.p.GetContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(cont1.IP, check.Equals, expectedIP)
	c.Assert(cont1.HostPort, check.Equals, expectedPort)
	c.Assert(cont1.Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	cont2, err = s.p.GetContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	expectedIP = dockerContainer.NetworkSettings.IPAddress
	expectedPort = dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(cont2.IP, check.Equals, expectedIP)
	c.Assert(cont2.HostPort, check.Equals, expectedPort)
	c.Assert(cont2.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestProvisionerStartProcess(c *check.C) {
	err := s.conn.Apps().Insert(&app.App{Name: "almah"})
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	err = s.p.Start(a, "web")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	cont1, err = s.p.GetContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(cont1.IP, check.Equals, expectedIP)
	c.Assert(cont1.HostPort, check.Equals, expectedPort)
	c.Assert(cont1.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestProvisionerStop(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	a := provisiontest.NewFakeApp("almah", "static", 2)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = dcli.StartContainer(cont1.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = dcli.StartContainer(cont2.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Stop(a, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerStopProcess(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	a := provisiontest.NewFakeApp("almah", "static", 2)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = dcli.StartContainer(cont1.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = dcli.StartContainer(cont2.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Stop(a, "worker")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerStopSkipAlreadyStoppedContainers(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	a := provisiontest.NewFakeApp("almah", "static", 2)
	container, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	container2, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	err = dcli.StartContainer(container2.ID, nil)
	c.Assert(err, check.IsNil)
	err = dcli.StopContainer(container2.ID, 1)
	c.Assert(err, check.IsNil)
	dockerContainer2, err := dcli.InspectContainer(container2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer2.State.Running, check.Equals, false)
	err = s.p.Stop(a, "")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer2, err = dcli.InspectContainer(container2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer2.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerSleep(c *check.C) {
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 2)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	err = dcli.StartContainer(cont1.ID, nil)
	c.Assert(err, check.IsNil)
	err = cont1.SetStatus(s.p.ClusterClient(), provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	err = dcli.StartContainer(cont2.ID, nil)
	c.Assert(err, check.IsNil)
	err = cont2.SetStatus(s.p.ClusterClient(), provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Sleep(a, "")
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Find(bson.M{"id": cont1.ID}).One(&cont1)
	c.Assert(err, check.IsNil)
	err = coll.Find(bson.M{"id": cont2.ID}).One(&cont2)
	c.Assert(err, check.IsNil)
	c.Assert(cont1.Status, check.Equals, provision.StatusAsleep.String())
	c.Assert(cont2.Status, check.Equals, provision.StatusAsleep.String())
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerSleepProcess(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	a := provisiontest.NewFakeApp("almah", "static", 2)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	cont1, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "web",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	err = cont1.SetStatus(s.p.ClusterClient(), provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = cont2.SetStatus(s.p.ClusterClient(), provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	err = dcli.StartContainer(cont1.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = dcli.StartContainer(cont2.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Sleep(a, "web")
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer, err = dcli.InspectContainer(cont2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
}

func (s *S) TestProvisionerUnits(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4f",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusBuilding.String(),
				IP:       "127.0.0.4",
				HostAddr: "192.168.123.9",
				HostPort: "9025",
			},
		},
	)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			Status:  provision.StatusBuilding,
			IP:      "192.168.123.9",
			Address: &url.URL{
				Scheme: "http",
				Host:   "192.168.123.9:9025",
			},
		},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerGetAppFromUnitID(c *check.C) {
	app := app.App{Name: "myapplication"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Insert(
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4f",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusBuilding.String(),
				IP:       "127.0.0.4",
				HostAddr: "192.168.123.9",
				HostPort: "9025",
			},
		},
	)
	c.Assert(err, check.IsNil)
	a, err := s.p.GetAppFromUnitID("9930c24f1c4f")
	c.Assert(err, check.IsNil)
	c.Assert(app.GetName(), check.Equals, a.GetName())
}

func (s *S) TestProvisionerGetAppFromUnitIDAppNotFound(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4f",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusBuilding.String(),
				IP:       "127.0.0.4",
				HostAddr: "192.168.123.9",
				HostPort: "9025",
			},
		},
	)
	c.Assert(err, check.IsNil)
	_, err = s.p.GetAppFromUnitID("9930c24f1c4f")
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerGetAppFromUnitIDContainerNotFound(c *check.C) {
	_, err := s.p.GetAppFromUnitID("not found")
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerUnitsAppDoesNotExist(c *check.C) {
	app := app.App{Name: "myapplication"}
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsStatus(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4f",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusBuilding.String(),
				IP:       "127.0.0.4",
				HostAddr: "10.0.0.7",
				HostPort: "9025",
			},
		},
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4j",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusError.String(),
				IP:       "127.0.0.4",
				HostAddr: "10.0.0.7",
				HostPort: "9025",
			},
		},
	)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	sortUnits(units)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			Status:  provision.StatusBuilding,
			IP:      "10.0.0.7",
			Address: &url.URL{
				Scheme: "http",
				Host:   "10.0.0.7:9025",
			},
		},
		{
			ID:      "9930c24f1c4j",
			AppName: "myapplication",
			Type:    "python",
			Status:  provision.StatusError,
			IP:      "10.0.0.7",
			Address: &url.URL{
				Scheme: "http",
				Host:   "10.0.0.7:9025",
			},
		},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsIp(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(
		container.Container{
			Container: types.Container{
				ID:       "9930c24f1c4f",
				AppName:  app.Name,
				Type:     "python",
				Status:   provision.StatusBuilding.String(),
				IP:       "127.0.0.4",
				HostPort: "9025",
				HostAddr: "127.0.0.1",
			},
		},
	)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			IP:      "127.0.0.1",
			Status:  provision.StatusBuilding,
			Address: &url.URL{
				Scheme: "http",
				Host:   "127.0.0.1:9025",
			},
		},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestRegisterUnit(c *check.C) {
	a := &app.App{Name: "myawesomeapp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarting.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.RegisterUnit(a, container.ID, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `\d+\.\d+\.\d+\.\d+`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestRegisterUnitBuildingContainer(c *check.C) {
	a := &app.App{Name: "myawesomeapp"}
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: a.Name}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.RegisterUnit(a, container.ID, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `xinvalidx`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusBuilding.String())
}

func (s *S) TestRegisterUnitSavesCustomDataRawProcfile(c *check.C) {
	a := &app.App{Name: "myawesomeapp"}
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: a.Name}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"mydata": "value", "procfile": "web: python myapp.py"}
	err = s.p.RegisterUnit(a, container.ID, data)
	c.Assert(err, check.IsNil)
	image, err := image.GetImageMetaData(container.BuildingImage)
	c.Assert(err, check.IsNil)
	c.Assert(image.CustomData, check.DeepEquals, data)
	expectedProcesses := map[string][]string{"web": {"python myapp.py"}}
	c.Assert(image.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestRegisterUnitSavesCustomDataParsedProcesses(c *check.C) {
	a := &app.App{Name: "myawesomeapp"}
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: a.Name}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{
		"mydata":   "value",
		"procfile": "web: python myapp.py",
		"processes": map[string]interface{}{
			"web":    "python web.py",
			"worker": "python worker.py",
		},
	}
	err = s.p.RegisterUnit(a, container.ID, data)
	c.Assert(err, check.IsNil)
	image, err := image.GetImageMetaData(container.BuildingImage)
	c.Assert(err, check.IsNil)
	c.Assert(image.CustomData, check.DeepEquals, data)
	expectedProcesses := map[string][]string{"web": {"python web.py"}, "worker": {"python worker.py"}}
	c.Assert(image.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestRegisterUnitInvalidProcfile(c *check.C) {
	a := &app.App{Name: "myawesomeapp"}
	err := newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: a.Name}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"mydata": "value", "procfile": "aaaaaaaaaaaaaaaaaaaaaa"}
	err = s.p.RegisterUnit(a, container.ID, data)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "invalid Procfile")
}

func (s *S) TestRunRestartAfterHooks(c *check.C) {
	a := &app.App{Name: "myrestartafterapp"}
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"after": []string{"cmd1", "cmd2"},
			},
		},
	}
	err := image.SaveImageCustomData("tsuru/python:latest", customData)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{AppName: a.Name}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	var reqBodies [][]byte
	s.server.CustomHandler("/containers/"+container.ID+"/exec", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		reqBodies = append(reqBodies, data)
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer container.Remove(s.p.ClusterClient(), s.p.ActionLimiter())
	var buf bytes.Buffer
	err = s.p.runRestartAfterHooks(container, &buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	c.Assert(reqBodies, check.HasLen, 2)
	var req1, req2 map[string]interface{}
	err = json.Unmarshal(reqBodies[0], &req1)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(reqBodies[1], &req2)
	c.Assert(err, check.IsNil)
	c.Assert(req1, check.DeepEquals, map[string]interface{}{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          []interface{}{"/bin/sh", "-lc", "cmd1"},
		"Container":    container.ID,
	})
	c.Assert(req2, check.DeepEquals, map[string]interface{}{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          []interface{}{"/bin/sh", "-lc", "cmd2"},
		"Container":    container.ID,
	})
}

func (s *S) TestExecuteCommandStdin(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-almah", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: conn,
		Stderr: conn,
		Stdin:  conn,
		Width:  10,
		Height: 10,
		Units:  []string{cont.ID},
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestDryMode(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	newProv, err := s.p.dryMode(nil)
	c.Assert(err, check.IsNil)
	contsNew, err := newProv.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(contsNew, check.HasLen, 5)
}

func (s *S) TestAddContainerDefaultProcess(c *check.C) {
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	err := newFakeImage(s.p, "tsuru/app-"+appName, customData)
	c.Assert(err, check.IsNil)
	s.p.Provision(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"": {Quantity: 2}},
		imageID:     "tsuru/app-" + appName,
	}
	containers, err := addContainersWithHost(&args)
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	c.Assert(parts[0], check.Equals, "")
	c.Assert(parts[1], check.Matches, `---- Starting 2 new units \[web: 2\] ----`)
	c.Assert(parts[2], check.Matches, ` ---> Started unit .+ \[web\]`)
	c.Assert(parts[3], check.Matches, ` ---> Started unit .+ \[web\]`)
	c.Assert(parts[4], check.Equals, "")
}

func (s *S) TestInitializeSetsBSHook(c *check.C) {
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(p.cluster, check.NotNil)
	c.Assert(p.cluster.Hooks(cluster.HookEventBeforeContainerCreate), check.DeepEquals, []cluster.Hook{&internalNodeContainer.ClusterHook{Provisioner: &p}})
}

func (s *S) TestProvisionerLogsEnabled(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	fakeApp.Pool = "mypool"
	tests := []struct {
		envs     []string
		poolEnvs map[string][]string
		enabled  bool
		msg      string
		err      error
	}{
		{nil, nil, true, "", nil},
		{[]string{}, nil, true, "", nil},
		{[]string{"LOG_BACKENDS=xxx"}, nil, false, "Logs not available through tsuru. Enabled log backends are:\n* xxx", nil},
		{[]string{"LOG_BACKENDS=xxx", "LOG_XXX_DOC=my doc"}, nil, false, "Logs not available through tsuru. Enabled log backends are:\n* xxx: my doc", nil},
		{[]string{"LOG_BACKENDS=a, b , c"}, nil, false, "Logs not available through tsuru. Enabled log backends are:\n* a\n* b\n* c", nil},
		{[]string{}, map[string][]string{"mypool": {"LOG_BACKENDS=abc"}}, false, "Logs not available through tsuru. Enabled log backends are:\n* abc", nil},
		{[]string{}, map[string][]string{"mypool": {"LOG_BACKENDS=abc", "LOG_ABC_DOC=doc"}}, false, "Logs not available through tsuru. Enabled log backends are:\n* abc: doc", nil},
		{[]string{}, map[string][]string{"otherpool": {"LOG_BACKENDS=abc"}}, true, "", nil},
		{[]string{}, map[string][]string{"mypool": {"LOG_BACKENDS=abc, tsuru "}}, true, "", nil},
	}
	for i, t := range tests {
		if t.envs != nil || t.poolEnvs != nil {
			err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
				Name: nodecontainer.BsDefaultName,
				Config: docker.Config{
					Env:   t.envs,
					Image: "img1",
				},
			})
			c.Assert(err, check.IsNil)
			for pool, envs := range t.poolEnvs {
				err := nodecontainer.AddNewContainer(pool, &nodecontainer.NodeContainerConfig{
					Name: nodecontainer.BsDefaultName,
					Config: docker.Config{
						Env: envs,
					},
				})
				c.Assert(err, check.IsNil)
			}
		}
		enabled, msg, err := s.p.LogsEnabled(fakeApp)
		c.Assert(err, check.Equals, t.err)
		c.Assert(enabled, check.Equals, t.enabled, check.Commentf("%d test", i))
		c.Assert(msg, check.Equals, t.msg)
		for pool := range t.poolEnvs {
			err = nodecontainer.RemoveContainer(pool, nodecontainer.BsDefaultName)
			c.Assert(err, check.IsNil)
		}
	}
}

func (s *S) TestProvisionerLogsEnabledOtherDriver(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	fakeApp.Pool = "mypool"
	logConf := container.DockerLogConfig{DockerLogConfig: types.DockerLogConfig{Driver: "x"}}
	err := logConf.Save("")
	c.Assert(err, check.IsNil)
	enabled, msg, err := s.p.LogsEnabled(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(enabled, check.Equals, false)
	c.Assert(msg, check.Equals, "Logs not available through tsuru. Enabled log driver is \"x\".")
	logConf = container.DockerLogConfig{DockerLogConfig: types.DockerLogConfig{Driver: "bs"}}
	err = logConf.Save("")
	c.Assert(err, check.IsNil)
	enabled, msg, err = s.p.LogsEnabled(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(enabled, check.Equals, true)
	c.Assert(msg, check.Equals, "")
}

func (s *S) TestProvisionerRoutableAddresses(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	routes, err := s.p.RoutableAddresses(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []url.URL{})
	err = image.AppendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = image.PullAppImageNames(appName, []string{"myimg"})
	c.Assert(err, check.IsNil)
	routes, err = s.p.RoutableAddresses(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []url.URL{})
	err = image.AppendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "myimg", nil)
	c.Assert(err, check.IsNil)
	conts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         fakeApp,
		imageID:     "myimg",
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	routes, err = s.p.RoutableAddresses(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []url.URL{
		*conts[0].Address(),
	})
}

func (s *S) TestProvisionerRoutableAddressesInvalidContainers(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	err := image.AppendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = newFakeImage(s.p, "myimg", nil)
	c.Assert(err, check.IsNil)
	conts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 3}},
		app:         fakeApp,
		imageID:     "myimg",
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 3)
	conts[0].HostAddr = ""
	conts[1].HostPort = ""
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": conts[0].ID}, conts[0])
	c.Assert(err, check.IsNil)
	err = coll.Update(bson.M{"id": conts[1].ID}, conts[1])
	c.Assert(err, check.IsNil)
	routes, err := s.p.RoutableAddresses(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []url.URL{
		*conts[2].Address(),
	})
}

func (s *S) TestFilterAppsByUnitStatus(c *check.C) {
	app1 := provisiontest.NewFakeApp("app1", "python", 0)
	app2 := provisiontest.NewFakeApp("app2", "python", 0)
	cont1, err := s.newContainer(&newContainerOpts{
		AppName: app1.GetName(),
		Status:  "stopped",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont1)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName: app2.GetName(),
		Status:  "started",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	apps, err := s.p.FilterAppsByUnitStatus([]provision.App{app1}, nil)
	c.Assert(apps, check.DeepEquals, []provision.App{})
	c.Assert(err, check.IsNil)
	apps, err = s.p.FilterAppsByUnitStatus(nil, []string{"building"})
	c.Assert(apps, check.IsNil)
	c.Assert(err, check.Not(check.IsNil))
	apps, err = s.p.FilterAppsByUnitStatus(nil, nil)
	c.Assert(apps, check.IsNil)
	c.Assert(err, check.Not(check.IsNil))
	apps, err = s.p.FilterAppsByUnitStatus([]provision.App{app1, app2}, []string{"started"})
	c.Assert(apps, check.DeepEquals, []provision.App{app2})
	c.Assert(err, check.IsNil)
	apps, err = s.p.FilterAppsByUnitStatus([]provision.App{app1, app2}, []string{"building"})
	c.Assert(apps, check.DeepEquals, []provision.App{})
	c.Assert(err, check.IsNil)
}

func (s *S) TestListNodes(c *check.C) {
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	listedNodes, err := s.p.ListNodes([]string{nodes[0].Address})
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{
		&clusterNodeWrapper{Node: &nodes[0], prov: s.p},
	})
	listedNodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{
		&clusterNodeWrapper{Node: &nodes[0], prov: s.p},
	})
	listedNodes, err = s.p.ListNodes([]string{"notfound"})
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{})
}

func (s *S) TestListNodesWithFilter(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	nodes, err := p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	filter := &provTypes.NodeFilter{Metadata: map[string]string{"pool": "test-default", "m1": "v1"}}
	listedNodes, err := p.ListNodesByFilter(filter)
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{
		&clusterNodeWrapper{Node: &nodes[0], prov: p},
	})
	filter = &provTypes.NodeFilter{Metadata: map[string]string{"pool": "test-default"}}
	listedNodes, err = p.ListNodesByFilter(filter)
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{
		&clusterNodeWrapper{Node: &nodes[0], prov: p},
		&clusterNodeWrapper{Node: &nodes[1], prov: p},
	})
	filter = &provTypes.NodeFilter{Metadata: map[string]string{"m1": "v1"}}
	listedNodes, err = p.ListNodesByFilter(filter)
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{
		&clusterNodeWrapper{Node: &nodes[0], prov: p},
	})
	filter = &provTypes.NodeFilter{Metadata: map[string]string{"m1": "v2"}}
	listedNodes, err = p.ListNodesByFilter(filter)
	c.Assert(err, check.IsNil)
	c.Assert(listedNodes, check.DeepEquals, []provision.Node{})
}

func (s *S) TestAddNode(c *check.C) {
	server, waitQueue := startFakeDockerNode(c)
	defer server.Stop()
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "")
	mainDockerProvisioner = &p
	opts := provision.AddNodeOptions{
		Address: server.URL(),
		Pool:    "pool1",
		Metadata: map[string]string{
			"m1": "x1",
		},
	}
	err = p.AddNode(opts)
	c.Assert(err, check.IsNil)
	waitQueue()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, server.URL())
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool":        "pool1",
		"m1":          "x1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusCreated)
}

func (s *S) TestAddRemoveAddNodeRace(c *check.C) {
	pong := make(chan struct{}, 2)
	var callCount int32
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		if strings.Contains(r.URL.Path, "ping") {
			pong <- struct{}{}
			if atomic.AddInt32(&callCount, 1) == 1 {
				time.Sleep(500 * time.Millisecond)
			}
		}
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "")
	mainDockerProvisioner = &p
	opts := provision.AddNodeOptions{
		Address: server.URL(),
		Pool:    "pool1",
		Metadata: map[string]string{
			"m1": "x1",
		},
	}
	err = p.AddNode(opts)
	c.Assert(err, check.IsNil)
	<-pong
	err = p.RemoveNode(provision.RemoveNodeOptions{
		Address: server.URL(),
	})
	c.Assert(err, check.IsNil)
	opts = provision.AddNodeOptions{
		Address: server.URL(),
		Pool:    "pool2",
		Metadata: map[string]string{
			"m2": "x2",
		},
	}
	err = p.AddNode(opts)
	c.Assert(err, check.IsNil)
	<-pong
	queue.ResetQueue()
	c.Assert(atomic.LoadInt32(&callCount), check.Equals, int32(2))
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, server.URL())
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool":        "pool2",
		"m2":          "x2",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusCreated)
}

func (s *S) TestAddNodeNoAddress(c *check.C) {
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "")
	mainDockerProvisioner = &p
	opts := provision.AddNodeOptions{}
	err = p.AddNode(opts)
	c.Assert(err, check.ErrorMatches, "Invalid address")
}

func (s *S) TestAddNodeWithWait(c *check.C) {
	server, _ := startFakeDockerNode(c)
	defer server.Stop()
	var p dockerProvisioner
	err := p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "")
	mainDockerProvisioner = &p
	opts := provision.AddNodeOptions{
		Address: server.URL(),
		Pool:    "pool1",
		WaitTO:  time.Second,
	}
	err = p.AddNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, server.URL())
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool":        "pool1",
		"LastSuccess": nodes[0].Metadata["LastSuccess"],
	})
	c.Assert(nodes[0].CreationStatus, check.Equals, cluster.NodeCreationStatusCreated)
}

func (s *S) TestRemoveNode(c *check.C) {
	var buf bytes.Buffer
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	opts := provision.RemoveNodeOptions{
		Address: nodes[0].Address,
		Writer:  &buf,
	}
	err = s.p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestRemoveNodeRebalanceNoUnits(c *check.C) {
	var buf bytes.Buffer
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	opts := provision.RemoveNodeOptions{
		Address:   nodes[0].Address,
		Rebalance: true,
		Writer:    &buf,
	}
	err = s.p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestRemoveNodeRebalanceWithUnits(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(net.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	opts := provision.RemoveNodeOptions{
		Address:   nodes[0].Address,
		Rebalance: true,
		Writer:    buf,
	}
	err = p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(net.URLToHost(nodes[0].Address), check.Equals, "localhost")
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 11)
	c.Assert(parts[0], check.Matches, `Moving unit .+? for "myapp" from 127\.0\.0\.1\.\.\.`)
	containerList, err := p.listContainersByHost(net.URLToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containerList, check.HasLen, 5)
}

func (s *S) TestRemoveNodeNoAddress(c *check.C) {
	var buf bytes.Buffer
	opts := provision.RemoveNodeOptions{
		Writer: &buf,
	}
	err := s.p.RemoveNode(opts)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
	c.Assert(buf.String(), check.Equals, "")
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestNodeUnits(c *check.C) {
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	units, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.DeepEquals, []provision.Unit{})
	err = newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	err = s.p.Provision(appInstance)
	c.Assert(err, check.IsNil)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	containers, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	units, err = nodes[0].Units()
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{
		containers[0].AsUnit(appInstance),
		containers[1].AsUnit(appInstance),
		containers[2].AsUnit(appInstance),
		containers[3].AsUnit(appInstance),
		containers[4].AsUnit(appInstance),
	}
	sortUnits(units)
	sortUnits(expected)
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestUpdateNode(c *check.C) {
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	opts := provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Metadata: map[string]string{
			"m1": "v1",
			"m2": "v2",
		},
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "waiting")
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"m1":   "v1",
		"m2":   "v2",
		"pool": "test-default",
	})
	opts = provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Metadata: map[string]string{
			"m1": "",
			"m3": "v3",
		},
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "waiting")
	c.Assert(nodes[0].Metadata, check.DeepEquals, map[string]string{
		"pool": "test-default",
		"m2":   "v2",
		"m3":   "v3",
	})
}

func (s *S) TestUpdateNodeDisableEnable(c *check.C) {
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	opts := provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Disable: true,
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	nodes, err = s.p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "disabled")
	opts = provision.UpdateNodeOptions{
		Address:  nodes[0].Address,
		Metadata: map[string]string{"a": "b"},
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "disabled")
	c.Assert(nodes[0].Metadata["a"], check.Equals, "b")
	opts = provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Enable:  true,
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err = s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "waiting")
	c.Assert(nodes[0].Metadata["a"], check.Equals, "b")
}

func (s *S) TestUpdateNodeNotFound(c *check.C) {
	opts := provision.UpdateNodeOptions{}
	err := s.p.UpdateNode(opts)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestUpdateNodeEnableCanMoveContainers(c *check.C) {
	nodes, err := s.p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	opts := provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Disable: true,
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	opts = provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Enable:  true,
	}
	err = s.p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	err = s.p.MoveContainers("localhost", "127.0.0.1", &buf)
	c.Assert(err, check.IsNil)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.DeepEquals, []string{
		"No units to move in localhost",
		"",
	})
}

func (s *S) TestUpdateNodeDisableCanMoveContainers(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(net.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	c.Assert(net.URLToHost(nodes[1].Address), check.Equals, "localhost")
	opts := provision.UpdateNodeOptions{
		Address: nodes[0].Address,
		Disable: true,
	}
	err = p.UpdateNode(opts)
	c.Assert(err, check.IsNil)
	err = p.MoveContainers("127.0.0.1", "localhost", buf)
	c.Assert(err, check.IsNil)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 4)
	c.Assert(parts[0], check.Equals, "Moving 1 units...")
	buf.Reset()
	err = p.MoveContainers("localhost", "127.0.0.1", buf)
	c.Assert(err, check.IsNil)
	parts = strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 6)
	c.Assert(parts[0], check.Equals, "Moving 2 units...")
}

func (s *S) TestNodeForNodeData(c *check.C) {
	err := newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	conts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	data := provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: conts[0].ID},
		},
	}
	node, err := s.p.NodeForNodeData(data)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, s.server.URL())
	data = provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{Name: conts[0].Name},
		},
	}
	node, err = s.p.NodeForNodeData(data)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, s.server.URL())
	data = provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: "invalidid"},
		},
	}
	_, err = s.p.NodeForNodeData(data)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRebalanceNodes(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeGlobal},
		Kind:    permission.PermNodeUpdateRebalance,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	toRebalance, err := p.RebalanceNodes(provision.RebalanceNodesOptions{
		Event:          evt,
		MetadataFilter: map[string]string{"pool": "test-default"},
	})
	c.Assert(err, check.IsNil, check.Commentf("Log: %s", buf.String()))
	c.Assert(toRebalance, check.Equals, true)
	c.Assert(buf.String(), check.Matches, "(?s).*Rebalancing as gap is 4, after rebalance gap will be 0.*Moving unit.*Moved unit.*")
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *S) TestRebalanceNodesCancel(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	blockCh := make(chan struct{})
	createCalled := make(chan struct{}, 2)
	s.extraServer.SetHook(func(r *http.Request) {
		if r.URL.Path == "/containers/create" {
			createCalled <- struct{}{}
			<-blockCh
		}
	})
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeGlobal},
		Kind:          permission.PermNodeUpdateRebalance,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermPoolReadEvents),
		Cancelable:    true,
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	done := make(chan bool)
	go func() {
		defer close(done)
		toRebalance, rebalanceErr := p.RebalanceNodes(provision.RebalanceNodesOptions{
			Event:          evt,
			MetadataFilter: map[string]string{"pool": "test-default"},
		})
		c.Assert(rebalanceErr, check.ErrorMatches, "(?s).*Caused by: unit creation canceled by user action.*")
		c.Assert(toRebalance, check.Equals, true)
	}()
	<-createCalled
	evtDB, err := event.GetByID(evt.UniqueID)
	c.Assert(err, check.IsNil)
	err = evtDB.TryCancel("because yes", "majortom@ground.control")
	c.Assert(err, check.IsNil)
	close(blockCh)
	<-done
	c.Assert(buf.String(), check.Matches, "(?s).*Rebalancing as gap is 4, after rebalance gap will be 0.*Moving unit.*")
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 4)
}

func (s *S) TestRebalanceNodesNoNeed(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	c1, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	c2, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeGlobal},
		Kind:    permission.PermNodeUpdateRebalance,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	toRebalance, err := p.RebalanceNodes(provision.RebalanceNodesOptions{
		Event:          evt,
		MetadataFilter: map[string]string{"pool": "test-default"},
	})
	c.Assert(err, check.IsNil, check.Commentf("Log: %s", buf.String()))
	c.Assert(toRebalance, check.Equals, false)
	c.Assert(buf.String(), check.Matches, "")
	conts, err := p.ListContainers(nil)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.Not(check.DeepEquals), append(c1, c2...))
}

func (s *S) TestRebalanceNodesNoNeedForce(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	c1, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	c2, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 2}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeGlobal},
		Kind:    permission.PermNodeUpdateRebalance,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	toRebalance, err := p.RebalanceNodes(provision.RebalanceNodesOptions{
		Event:          evt,
		Force:          true,
		MetadataFilter: map[string]string{"pool": "test-default"},
	})
	c.Assert(err, check.IsNil, check.Commentf("Log: %s", buf.String()))
	c.Assert(toRebalance, check.Equals, true)
	c.Assert(buf.String(), check.Matches, "(?s).*Rebalancing 4 units.*Moving unit.*Moved unit.*")
	conts, err := p.ListContainers(nil)
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.Not(check.DeepEquals), append(c1, c2...))
}

func (s *S) TestRebalanceNodesDry(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = p
	err = newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(appInstance)
	imageID, err := image.AppCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         appInstance,
		imageID:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	appStruct := s.newAppFromFake(appInstance)
	appStruct.Pool = "test-default"
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeGlobal},
		Kind:    permission.PermNodeUpdateRebalance,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	toRebalance, err := p.RebalanceNodes(provision.RebalanceNodesOptions{
		Event:          evt,
		Dry:            true,
		MetadataFilter: map[string]string{"pool": "test-default"},
	})
	c.Assert(err, check.IsNil, check.Commentf("Log: %s", buf.String()))
	c.Assert(toRebalance, check.Equals, true)
	c.Assert(buf.String(), check.Matches, "(?s).*Rebalancing as gap is 4, after rebalance gap will be 0.*Would move unit.*")
	containers, err := p.listContainersByHost("localhost")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	containers, err = p.listContainersByHost("127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 4)
}
