// Copyright 2016 tsuru authors. All rights reserved.
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
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/external/github.com/docker/docker/pkg/stdcopy"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

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

func (s *S) TestProvisionerRestartRebuildRoutesInQueue(c *check.C) {
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	coll := s.storage.Apps()
	a := &app.App{
		Name:     "almah",
		Platform: "static",
	}
	err = coll.Insert(a)
	c.Assert(err, check.IsNil)
	err = s.p.Provision(a)
	c.Assert(err, check.IsNil)
	imageId := "tsuru/app-" + a.Name + ":v1"
	err = s.newFakeImage(s.p, imageId, nil)
	c.Assert(err, check.IsNil)
	_, err = s.p.runCreateUnitsPipeline(nil, a, map[string]*containersToAdd{"web": {Quantity: 1}}, imageId)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(a.GetName(), invalidAddr)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp(invalidAddr.String())
	err = s.p.Restart(a, "", nil)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, true)
	routertest.FakeRouter.RemoveFailForIp(invalidAddr.String())
	err = queue.TestingWaitQueueTasks(1, 10*time.Second)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
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
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Quota{Limit: 10, InUse: 10},
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
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
	err = saveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].Ip)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, quota.Quota{Limit: 10, InUse: 1})
}

func (s *S) TestDeployQuotaExceeded(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Quota{Limit: 1},
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
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
	err = saveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.NotNil)
	compErr, ok := err.(*errors.CompositeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(compErr.Message, check.Equals, "Cannot start application units")
	e, ok := compErr.Base.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestDeployErasesOldImages(c *check.C) {
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	stopCh := s.stopContainers(s.server.URL(), 3)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository(a.Name, nil)
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = saveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	err = saveImageCustomData("tsuru/app-"+a.Name+":v2", customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	expected := []string{"tsuru/app-appdeployimagetest:v1", "tsuru/python:latest"}
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v2", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestDeployErasesOldImagesIfFailed(c *check.C) {
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var result docker.Config
		jsonErr := json.Unmarshal(data, &result)
		if jsonErr == nil {
			if result.Image == "tsuru/python:latest" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Commit:       "123",
		OutputStream: w,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
	})
	c.Assert(err, check.NotNil)
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/python:latest", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestDeployErasesOldImagesWithLongHistory(c *check.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	stopCh := s.stopContainers(s.server.URL(), 4)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository(a.Name, nil)
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = saveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	err = saveImageCustomData("tsuru/app-"+a.Name+":v2", customData)
	c.Assert(err, check.IsNil)
	err = saveImageCustomData("tsuru/app-"+a.Name+":v3", customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	expected := []string{"tsuru/app-appdeployimagetest:v1", "tsuru/python:latest"}
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 3)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	c.Assert(imgs[2].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0], imgs[2].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v1", "tsuru/app-appdeployimagetest:v2", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 3)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	c.Assert(imgs[2].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0], imgs[2].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v2", "tsuru/app-appdeployimagetest:v3", "tsuru/python:latest"}
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUploadDeploy(c *check.C) {
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, nil, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	buf := bytes.NewBufferString("something wrong is not right")
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = saveImageCustomData("tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		File:         ioutil.NopCloser(buf),
		FileSize:     int64(buf.Len()),
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].Ip)
}

func (s *S) TestRollbackDeploy(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-otherapp:v1", nil)
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
		Rollback:     true,
	})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRollbackDeployFailureDoesntEraseImage(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-otherapp:v1", nil)
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
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
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
		Rollback:     true,
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

func (s *S) TestImageDeploy(c *check.C) {
	p, err := s.startMultipleServersClusterSeggregated()
	c.Assert(err, check.IsNil)
	app.Provisioner = p
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
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
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler(fmt.Sprintf("/images/%s/json", imageName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{"/bin/sh", "-c", "python test.py"},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	customData := map[string]interface{}{}
	err = s.newFakeImage(p, imageName, customData)
	c.Assert(err, check.IsNil)
	pushOpts := docker.PushImageOptions{
		Name:     imageName,
		Registry: s.server.URL(),
	}
	err = p.Cluster().PushImage(pushOpts, mainDockerProvisioner.RegistryAuthConfig())
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Unlimited,
		Pool:     "pool1",
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	p.Provision(&a)
	defer s.p.Destroy(&a)
	dataColl, err := imageCustomDataColl()
	defer dataColl.Close()
	c.Assert(err, check.IsNil)
	dataColl.RemoveId(imageName)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        imageName,
	})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	appCurrentImage, err := appCurrentImageName(a.GetName())
	imd, err := getImageCustomData(appCurrentImage)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string]string{"web": "/bin/sh \"-c\" \"python test.py\""}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
	updatedApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.GetUpdatePlatform(), check.Equals, true)
}

func (s *S) TestImageDeployWithProcfile(c *check.C) {
	p, err := s.startMultipleServersClusterSeggregated()
	c.Assert(err, check.IsNil)
	app.Provisioner = p
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
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
		fmt.Fprintf(outStream, "web: test.sh\n")
		conn.Close()
	}))
	customData := map[string]interface{}{}
	err = s.newFakeImage(p, imageName, customData)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Unlimited,
		Pool:     "pool1",
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	p.Provision(&a)
	defer p.Destroy(&a)
	dataColl, err := imageCustomDataColl()
	defer dataColl.Close()
	c.Assert(err, check.IsNil)
	dataColl.RemoveId(imageName)
	defer dataColl.Close()
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        imageName,
	})
	c.Assert(err, check.IsNil)
	appCurrentImage, err := appCurrentImageName(a.GetName())
	imd, err := getImageCustomData(appCurrentImage)
	c.Assert(err, check.IsNil)
	expectedProcesses := map[string]string{"web": "test.sh"}
	c.Assert(imd.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestImageDeployShouldHaveAnEntrypoint(c *check.C) {
	p, err := s.startMultipleServersClusterSeggregated()
	c.Assert(err, check.IsNil)
	app.Provisioner = p
	u, _ := url.Parse(s.server.URL())
	imageName := fmt.Sprintf("%s/%s", u.Host, "customimage")
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
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
		fmt.Fprintf(outStream, "")
		conn.Close()
	}))
	s.server.CustomHandler("/images/.*/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint: []string{},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	customData := map[string]interface{}{}
	err = s.newFakeImage(p, imageName, customData)
	c.Assert(err, check.IsNil)
	pushOpts := docker.PushImageOptions{
		Name:     imageName,
		Registry: s.server.URL(),
	}
	err = p.Cluster().PushImage(pushOpts, mainDockerProvisioner.RegistryAuthConfig())
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
		Quota:    quota.Unlimited,
		Pool:     "pool1",
	}
	err = s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	p.Provision(&a)
	defer p.Destroy(&a)
	dataColl, err := imageCustomDataColl()
	defer dataColl.Close()
	c.Assert(err, check.IsNil)
	dataColl.RemoveId(imageName)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        imageName,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrEntrypointOrProcfileNotFound)
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

func (s *S) TestProvisionerDestroyRemovesImage(c *check.C) {
	var registryRequests []*http.Request
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		registryRequests = append(registryRequests, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer registryServer.Close()
	registryURL := strings.Replace(registryServer.URL, "http://", "", 1)
	config.Set("docker:registry", registryURL)
	defer config.Unset("docker:registry")
	stopCh := s.stopContainers(s.server.URL(), 1)
	defer func() { <-stopCh }()
	a := app.App{
		Name:     "mydoomedapp",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = saveImageCustomData(fmt.Sprintf("%s/tsuru/app-%s:v1", registryURL, a.Name), customData)
	c.Assert(err, check.IsNil)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		ArchiveURL:   "https://mystorage.com/archive.tar.gz",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(&a)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	count, err := coll.Find(bson.M{"appname": a.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, false)
	c.Assert(registryRequests, check.HasLen, 1)
	c.Assert(registryRequests[0].Method, check.Equals, "DELETE")
	c.Assert(registryRequests[0].URL.Path, check.Equals, "/v1/repositories/tsuru/app-mydoomedapp:v1/")
	imgs, err := s.p.Cluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags[0], check.Equals, registryURL+"/tsuru/python:latest")
}

func (s *S) TestProvisionerDestroyEmptyUnit(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	err := s.p.Destroy(a)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerDestroyRemovesRouterBackend(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, false)
}

func (s *S) TestProvisionerAddr(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	a := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	addr, err := s.p.Addr(a)
	c.Assert(err, check.IsNil)
	r, err := getRouterForApp(a)
	c.Assert(err, check.IsNil)
	expected, err := r.Addr(cont.AppName)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	defer s.p.Destroy(a)
	_, err = s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	defer coll.RemoveAll(bson.M{"appname": a.GetName()})
	c.Assert(units, check.HasLen, 3)
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 4)
}

func (s *S) TestProvisionerAddUnitsInvalidProcess(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	defer s.p.Destroy(a)
	_, err = s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnits(a, 3, "bogus", nil)
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
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	defer s.p.Destroy(a)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "c-89320", AppName: a.GetName(), Version: "a345fe", Image: "tsuru/python:latest"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	_, err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.NotNil)
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestProvisionerAddZeroUnits(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	a.Deploys = 1
	s.p.Provision(a)
	defer s.p.Destroy(a)
	coll := s.p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "c-89320", AppName: a.GetName(), Version: "a345fe", Image: "tsuru/python:latest"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	units, err := s.p.AddUnits(a, 0, "web", nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units")
}

func (s *S) TestProvisionerAddUnitsWithNoDeploys(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	s.p.Provision(a)
	defer s.p.Destroy(a)
	units, err := s.p.AddUnits(a, 1, "web", nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "New units can only be added after the first deployment")
}

func (s *S) TestProvisionerAddUnitsWithHost(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(a)
	defer p.Destroy(a)
	coll := p.Collection()
	defer coll.Close()
	coll.Insert(container.Container{ID: "xxxfoo", AppName: a.GetName(), Version: "123987", Image: "tsuru/python:latest"})
	defer coll.RemoveId(bson.M{"id": "xxxfoo"})
	imageID, err := appCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         a,
		imageId:     imageID,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": a.GetName()})
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].HostAddr, check.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 2)
}

func (s *S) TestProvisionerAddUnitsWithHostPartialRollback(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(a)
	defer s.p.Destroy(a)
	imageId, err := appCurrentImageName(a.GetName())
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
		imageId:     imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.ErrorMatches, "error in docker node.*")
	c.Assert(units, check.HasLen, 0)
	coll := s.p.Collection()
	defer coll.Close()
	count, err := coll.Find(bson.M{"appname": a.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestProvisionerRemoveUnits(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "1"}
	cont2 := container.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "worker", HostAddr: "url0", HostPort: "2"}
	cont3 := container.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "3"}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	o := provision.AddPoolOptions{Name: p.Name}
	err = provision.AddPool(o)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage)
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
	err = saveImageCustomData("tsuru/app-"+a1.Name, customData)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	conts := []container.Container{cont1, cont2, cont3}
	units := []provision.Unit{cont1.AsUnit(papp), cont2.AsUnit(papp), cont3.AsUnit(papp)}
	for i := range conts {
		err = routertest.FakeRouter.AddRoute(a1.Name, conts[i].Address())
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
	cont1 := container.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "1"}
	cont2 := container.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "worker", HostAddr: "url0", HostPort: "2"}
	cont3 := container.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web", HostAddr: "url0", HostPort: "3"}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	o := provision.AddPoolOptions{Name: p.Name}
	err = provision.AddPool(o)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage)
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
	err = saveImageCustomData("tsuru/app-"+a1.Name, customData)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	conts := []container.Container{cont1, cont2, cont3}
	units := []provision.Unit{cont1.AsUnit(papp), cont2.AsUnit(papp), cont3.AsUnit(papp)}
	for i := range conts {
		err = routertest.FakeRouter.AddRoute(a1.Name, conts[i].Address())
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
	cont1 := container.Container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
	}}
	o := provision.AddPoolOptions{Name: p.Name}
	err = provision.AddPool(o)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage)
	c.Assert(err, check.IsNil)
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	err = clusterInstance.Register(cluster.Node{
		Address:  s.server.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, []string{a1.Name, "web"})
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
	cont1 := container.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web"}
	cont2 := container.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "web"}
	cont3 := container.Container{ID: "3", Name: "dedication1", AppName: a1.Name, ProcessName: "web"}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	o := provision.AddPoolOptions{Name: p.Name}
	err = provision.AddPool(o)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage)
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
	err = saveImageCustomData("tsuru/app-"+a1.Name, customData)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnits(papp, 4, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cannot remove 4 units from process \"web\", only 3 available")
}

func (s *S) TestProvisionerRemoveUnitsInvalidProcess(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	cont1 := container.Container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
	}}
	o := provision.AddPoolOptions{Name: p.Name}
	err = provision.AddPool(o)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	s.p.storage = &cluster.MapStorage{}
	clusterInstance, err := cluster.New(&scheduler, s.p.storage)
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	err = clusterInstance.Register(cluster.Node{
		Address:  s.server.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, []string{a1.Name, "web"})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	err = saveImageCustomData("tsuru/app-"+a1.Name, customData)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnits(papp, 1, "worker", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `process error: no command declared in Procfile for process "worker"`)
}

func (s *S) TestProvisionerSetUnitStatus(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
}

func (s *S) TestProvisionerSetUnitStatusAsleep(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = container.Sleep(s.p)
	c.Assert(err, check.IsNil)
	err = s.p.SetUnitStatus(provision.Unit{ID: container.ID, AppName: container.AppName}, provision.StatusStopped)
	c.Assert(err, check.IsNil)
	container, err = s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusAsleep.String())
}

func (s *S) TestProvisionerSetUnitStatusUpdatesIp(c *check.C) {
	err := s.storage.Apps().Insert(&app.App{Name: "myawesomeapp"})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err = s.p.ExecuteCommand(&stdout, &stderr, a, "ls", "-l")
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *check.C) {
	a := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := s.p.ExecuteCommand(&buf, &buf, a, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionerExecuteCommandExcludesBuildContainers(c *check.C) {
	a := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container2, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container3, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container4, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container2.SetStatus(s.p, provision.StatusCreated, true)
	container3.SetStatus(s.p, provision.StatusBuilding, true)
	container4.SetStatus(s.p, provision.StatusStopped, true)
	containers := []*container.Container{
		container1,
		container2,
		container3,
		container4,
	}
	coll := s.p.Collection()
	defer coll.Close()
	for _, c := range containers {
		defer s.removeTestContainer(c)
	}
	var executed int
	s.server.PrepareExec("*", func() {
		executed++
	})
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(&stdout, &stderr, a, "echo x")
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, 1)
}

func (s *S) TestProvisionerExecuteCommandOnce(c *check.C) {
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
	err = s.p.ExecuteCommandOnce(&stdout, &stderr, a, "ls", "-l")
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, true)
}

func (s *S) TestProvisionerExecuteCommandOnceNoContainers(c *check.C) {
	a := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := s.p.ExecuteCommandOnce(&buf, &buf, a, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionCollection(c *check.C) {
	collection := s.p.Collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, s.collName)
}

func (s *S) TestProvisionSetCName(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	addr, _ := url.Parse("http://127.0.0.1")
	routertest.FakeRouter.AddRoute("myapp", addr)
	cname := "mycname.com"
	err := s.p.SetCName(a, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasCName(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, addr.String()), check.Equals, true)
}

func (s *S) TestProvisionUnsetCName(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	addr, _ := url.Parse("http://127.0.0.1")
	routertest.FakeRouter.AddRoute("myapp", addr)
	cname := "mycname.com"
	err := s.p.SetCName(a, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasCName(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, addr.String()), check.Equals, true)
	err = s.p.UnsetCName(a, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasCName(cname), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(cname, addr.String()), check.Equals, false)
}

func (s *S) TestProvisionerIsCNameManager(c *check.C) {
	var _ provision.CNameManager = &dockerProvisioner{}
}

func (s *S) TestAdminCommands(c *check.C) {
	expected := []cmd.Command{
		&moveContainerCmd{},
		&moveContainersCmd{},
		&rebalanceContainersCmd{},
		&addNodeToSchedulerCmd{},
		&removeNodeFromSchedulerCmd{},
		&listNodesInTheSchedulerCmd{},
		&healer.ListHealingHistoryCmd{},
		&healer.GetNodeHealingConfigCmd{},
		&healer.SetNodeHealingConfigCmd{},
		&healer.DeleteNodeHealingConfigCmd{},
		&autoScaleRunCmd{},
		&listAutoScaleHistoryCmd{},
		&autoScaleInfoCmd{},
		&autoScaleSetRuleCmd{},
		&autoScaleDeleteRuleCmd{},
		&updateNodeToSchedulerCmd{},
		&dockerLogInfo{},
		&dockerLogUpdate{},
		&nodecontainer.NodeContainerList{},
		&nodecontainer.NodeContainerAdd{},
		&nodecontainer.NodeContainerInfo{},
		&nodecontainer.NodeContainerUpdate{},
		&nodecontainer.NodeContainerDelete{},
		&nodecontainer.NodeContainerUpgrade{},
		&cmd.RemovedCommand{Name: "bs-env-set", Help: "You should use `tsuru-admin node-container-update big-sibling` instead."},
		&cmd.RemovedCommand{Name: "bs-info", Help: "You should use `tsuru-admin node-container-info big-sibling` instead."},
		&cmd.RemovedCommand{Name: "bs-upgrade", Help: "You should use `tsuru-admin node-container-upgrade big-sibling` instead."},
	}
	c.Assert(s.p.AdminCommands(), check.DeepEquals, expected)
}

func (s *S) TestProvisionerIsAdminCommandable(c *check.C) {
	var _ cmd.AdminCommandable = &dockerProvisioner{}
}

func (s *S) TestSwap(c *check.C) {
	app1 := provisiontest.NewFakeApp("app1", "python", 1)
	app2 := provisiontest.NewFakeApp("app2", "python", 1)
	routertest.FakeRouter.AddBackend(app1.GetName())
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://127.0.0.2")
	routertest.FakeRouter.AddRoute(app1.GetName(), addr1)
	routertest.FakeRouter.AddBackend(app2.GetName())
	routertest.FakeRouter.AddRoute(app2.GetName(), addr2)
	err := s.p.Swap(app1, app2)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app1.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasBackend(app2.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app2.GetName(), addr1.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app1.GetName(), addr2.String()), check.Equals, true)
}

func (s *S) TestProvisionerStart(c *check.C) {
	err := s.storage.Apps().Insert(&app.App{Name: "almah"})
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
	err := s.storage.Apps().Insert(&app.App{Name: "almah"})
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
	err = cont1.SetStatus(s.p, provision.StatusStarted, true)
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
	err = cont2.SetStatus(s.p, provision.StatusStarted, true)
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
	err = cont1.SetStatus(s.p, provision.StatusStarted, true)
	c.Assert(err, check.IsNil)
	cont2, err := s.newContainer(&newContainerOpts{
		AppName:         a.GetName(),
		Image:           "tsuru/app-" + a.GetName(),
		ImageCustomData: customData,
		ProcessName:     "worker",
	}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont2)
	err = cont2.SetStatus(s.p, provision.StatusStarted, true)
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

func (s *S) TestProvisionerPlatformAdd(c *check.C) {
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = p.PlatformAdd(provision.PlatformOptions{
		Name:   "test",
		Args:   args,
		Output: ioutil.Discard,
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(requests) >= 3, check.Equals, true)
	requests = requests[len(requests)-3:]
	c.Assert(requests[0].URL.Path, check.Equals, "/build")
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), check.Equals, platformImageName("test"))
	c.Assert(queryString.Get("remote"), check.Equals, "http://localhost/Dockerfile")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test:latest/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test/push")
}

func (s *S) TestProvisionerPlatformAddData(c *check.C) {
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, cluster.Node{Address: server.URL()})
	dockerfile := "FROM tsuru/java"
	err = p.PlatformAdd(provision.PlatformOptions{
		Name:   "test",
		Args:   map[string]string{"dockerfile": "http://localhost"},
		Output: ioutil.Discard,
		Input:  strings.NewReader(dockerfile),
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(requests) >= 3, check.Equals, true)
	requests = requests[len(requests)-3:]
	c.Assert(requests[0].URL.Path, check.Equals, "/build")
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), check.Equals, platformImageName("test"))
	c.Assert(queryString.Get("remote"), check.Equals, "")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test:latest/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test/push")
}

func (s *S) TestProvisionerPlatformAddWithoutArgs(c *check.C) {
	err := s.p.PlatformAdd(provision.PlatformOptions{Name: "test"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Dockerfile is required")
}

func (s *S) TestProvisionerPlatformAddShouldValidateArgs(c *check.C) {
	args := make(map[string]string)
	args["dockerfile"] = "not_a_url"
	err := s.p.PlatformAdd(provision.PlatformOptions{
		Name:   "test",
		Args:   args,
		Output: ioutil.Discard,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "dockerfile parameter must be a URL")
}

func (s *S) TestProvisionerPlatformAddWithoutNode(c *check.C) {
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{})
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = p.PlatformAdd(provision.PlatformOptions{
		Name:   "test",
		Args:   args,
		Output: ioutil.Discard,
	})
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerPlatformRemove(c *check.C) {
	registryServer := httptest.NewServer(nil)
	defer registryServer.Close()
	u, _ := url.Parse(registryServer.URL)
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	var buf bytes.Buffer
	err = p.PlatformAdd(provision.PlatformOptions{
		Name:   "test",
		Args:   map[string]string{"dockerfile": "http://localhost/Dockerfile"},
		Output: &buf,
	})
	c.Assert(err, check.IsNil)
	err = p.PlatformRemove("test")
	c.Assert(err, check.IsNil)
	c.Assert(len(requests) >= 4, check.Equals, true)
	requests = requests[len(requests)-4:]
	c.Assert(requests[3].Method, check.Equals, "DELETE")
	c.Assert(requests[3].URL.Path, check.Matches, "/images/[^/]+")
}

func (s *S) TestProvisionerPlatformRemoveReturnsStorageError(c *check.C) {
	registryServer := httptest.NewServer(nil)
	defer registryServer.Close()
	u, _ := url.Parse(registryServer.URL)
	config.Set("docker:registry", u.Host)
	defer config.Unset("docker:registry")
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var strg cluster.MapStorage
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster, _ = cluster.New(nil, &strg,
		cluster.Node{Address: server.URL()})
	err = p.PlatformRemove("test")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.DeepEquals, storage.ErrNoSuchImage)
}

func (s *S) TestProvisionerUnits(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.Collection()
	defer coll.Close()
	err := coll.Insert(
		container.Container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostAddr: "192.168.123.9",
			HostPort: "9025",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			Status:  provision.StatusBuilding,
			Ip:      "192.168.123.9",
			Address: &url.URL{
				Scheme: "http",
				Host:   "192.168.123.9:9025",
			},
		},
	}
	c.Assert(units, check.DeepEquals, expected)
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
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostAddr: "10.0.0.7",
			HostPort: "9025",
		},
		container.Container{
			ID:       "9930c24f1c4j",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusError.String(),
			IP:       "127.0.0.4",
			HostAddr: "10.0.0.7",
			HostPort: "9025",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	sortUnits(units)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			Status:  provision.StatusBuilding,
			Ip:      "10.0.0.7",
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
			Ip:      "10.0.0.7",
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
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
			HostAddr: "127.0.0.1",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	units, err := s.p.Units(&app)
	c.Assert(err, check.IsNil)
	expected := []provision.Unit{
		{
			ID:      "9930c24f1c4f",
			AppName: "myapplication",
			Type:    "python",
			Ip:      "127.0.0.1",
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
	err := s.storage.Apps().Insert(&app.App{Name: "myawesomeapp"})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "tsuru/python:latest", nil)
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
	err = s.p.RegisterUnit(provision.Unit{ID: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `\d+\.\d+\.\d+\.\d+`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestRegisterUnitBuildingContainer(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.RegisterUnit(provision.Unit{ID: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.GetContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `xinvalidx`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusBuilding.String())
}

func (s *S) TestRegisterUnitSavesCustomDataRawProcfile(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
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
	err = s.p.RegisterUnit(provision.Unit{ID: container.ID}, data)
	c.Assert(err, check.IsNil)
	dataColl, err := imageCustomDataColl()
	c.Assert(err, check.IsNil)
	defer dataColl.Close()
	var image ImageMetadata
	err = dataColl.FindId(container.BuildingImage).One(&image)
	c.Assert(err, check.IsNil)
	c.Assert(image.CustomData, check.DeepEquals, data)
	expectedProcesses := map[string]string{"web": "python myapp.py"}
	c.Assert(image.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestRegisterUnitSavesCustomDataParsedProcesses(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
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
	err = s.p.RegisterUnit(provision.Unit{ID: container.ID}, data)
	c.Assert(err, check.IsNil)
	dataColl, err := imageCustomDataColl()
	c.Assert(err, check.IsNil)
	defer dataColl.Close()
	var image ImageMetadata
	err = dataColl.FindId(container.BuildingImage).One(&image)
	c.Assert(err, check.IsNil)
	c.Assert(image.CustomData, check.DeepEquals, data)
	expectedProcesses := map[string]string{"web": "python web.py", "worker": "python worker.py"}
	c.Assert(image.Processes, check.DeepEquals, expectedProcesses)
}

func (s *S) TestRegisterUnitInvalidProcfile(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python:latest", nil)
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"mydata": "value", "procfile": "web: python myapp.py\nworker:python worker.py"}
	err = s.p.RegisterUnit(provision.Unit{ID: container.ID}, data)
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
	err := saveImageCustomData("tsuru/python:latest", customData)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Insert(a)
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
	defer container.Remove(s.p)
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
		"Cmd":          []interface{}{"/bin/bash", "-lc", "cmd1"},
		"Container":    container.ID,
	})
	c.Assert(req2, check.DeepEquals, map[string]interface{}{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          []interface{}{"/bin/bash", "-lc", "cmd2"},
		"Container":    container.ID,
	})
}

func (s *S) TestShellToAnAppByContainerID(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-almah", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ShellOptions{App: a, Conn: conn, Width: 10, Height: 10, Unit: cont.ID}
	err = s.p.Shell(opts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShellToAnAppByAppName(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-almah", nil)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: a.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ShellOptions{App: a, Conn: conn, Width: 10, Height: 10}
	err = s.p.Shell(opts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDryMode(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp", nil)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer s.p.Destroy(appInstance)
	s.p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 5}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	newProv, err := s.p.dryMode(nil)
	c.Assert(err, check.IsNil)
	contsNew, err := newProv.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(contsNew, check.HasLen, 5)
}

func (s *S) TestMetricEnvs(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: nodecontainer.BsDefaultName,
		Config: docker.Config{
			Image: "img1",
			Env: []string{
				"OTHER_ENV=asd",
				"METRICS_BACKEND=LOGSTASH",
				"METRICS_LOGSTASH_URI=localhost:2222",
			},
		},
	})
	c.Assert(err, check.IsNil)
	appInstance := &app.App{
		Name: "impius",
		Pool: "mypool",
	}
	envs := s.p.MetricEnvs(appInstance)
	expected := map[string]string{
		"METRICS_LOGSTASH_URI": "localhost:2222",
		"METRICS_BACKEND":      "LOGSTASH",
	}
	c.Assert(envs, check.DeepEquals, expected)
}

func (s *S) TestAddContainerDefaultProcess(c *check.C) {
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	}
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	err := s.newFakeImage(s.p, "tsuru/app-"+appName, customData)
	c.Assert(err, check.IsNil)
	s.p.Provision(fakeApp)
	defer s.p.Destroy(fakeApp)
	buf := safe.NewBuffer(nil)
	args := changeUnitsPipelineArgs{
		app:         fakeApp,
		provisioner: s.p,
		writer:      buf,
		toAdd:       map[string]*containersToAdd{"": {Quantity: 2}},
		imageId:     "tsuru/app-" + appName,
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
	c.Assert(p.cluster.Hooks(cluster.HookEventBeforeContainerCreate), check.DeepEquals, []cluster.Hook{&nodecontainer.ClusterHook{Provisioner: &p}})
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
	logConf := container.DockerLogConfig{Driver: "x"}
	err := logConf.Save("")
	c.Assert(err, check.IsNil)
	enabled, msg, err := s.p.LogsEnabled(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(enabled, check.Equals, false)
	c.Assert(msg, check.Equals, "Logs not available through tsuru. Enabled log driver is \"x\".")
	logConf = container.DockerLogConfig{Driver: "bs"}
	err = logConf.Save("")
	c.Assert(err, check.IsNil)
	enabled, msg, err = s.p.LogsEnabled(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(enabled, check.Equals, true)
	c.Assert(msg, check.Equals, "")
}

func (s *S) TestProvisionerRoutableUnits(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	routes, err := s.p.RoutableUnits(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []provision.Unit{})
	err = appendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = pullAppImageNames(appName, []string{"myimg"})
	c.Assert(err, check.IsNil)
	routes, err = s.p.RoutableUnits(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []provision.Unit{})
	err = appendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "myimg", nil)
	c.Assert(err, check.IsNil)
	conts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         fakeApp,
		imageId:     "myimg",
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	routes, err = s.p.RoutableUnits(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []provision.Unit{
		conts[0].AsUnit(fakeApp),
	})
}

func (s *S) TestProvisionerRoutableUnitsInvalidContainers(c *check.C) {
	appName := "my-fake-app"
	fakeApp := provisiontest.NewFakeApp(appName, "python", 0)
	err := appendAppImageName(appName, "myimg")
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "myimg", nil)
	c.Assert(err, check.IsNil)
	conts, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 3}},
		app:         fakeApp,
		imageId:     "myimg",
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
	routes, err := s.p.RoutableUnits(fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []provision.Unit{
		conts[2].AsUnit(fakeApp),
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
