// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
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
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	err = s.p.Start(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err := s.p.getCluster().InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Restart(app, nil)
	c.Assert(err, check.IsNil)
	dbConts, err := s.p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 1)
	c.Assert(dbConts[0].ID, check.Not(check.Equals), cont.ID)
	c.Assert(dbConts[0].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[0].Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = s.p.getCluster().InspectContainer(dbConts[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(dbConts[0].IP, check.Equals, expectedIP)
	c.Assert(dbConts[0].HostPort, check.Equals, expectedPort)
}

func (s *S) stopContainers(n uint) {
	client, err := docker.NewClient(s.server.URL())
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
}

func (s *S) TestDeploy(c *check.C) {
	go s.stopContainers(1)
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	units := a.Units()
	c.Assert(units, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].Ip)
}

func (s *S) TestDeployErasesOldImages(c *check.C) {
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	go s.stopContainers(3)
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	repository.Manager().CreateRepository(a.Name, nil)
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	expected := []string{"tsuru/app-appdeployimagetest:v1", "tsuru/python"}
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v2", "tsuru/python"}
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestDeployErasesOldImagesIfFailed(c *check.C) {
	config.Set("docker:image-history-size", 1)
	defer config.Unset("docker:image-history-size")
	go s.stopContainers(1)
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var result docker.Config
		err := json.Unmarshal(data, &result)
		if err == nil {
			if result.Image == "tsuru/app-appdeployimagetest:v1" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.NotNil)
	imgs, err := s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/python", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestDeployErasesOldImagesWithLongHistory(c *check.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	go s.stopContainers(5)
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	repository.Manager().CreateRepository(a.Name, nil)
	err = s.p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 2)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	expected := []string{"tsuru/app-appdeployimagetest:v1", "tsuru/python"}
	got := []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0]}
	sort.Strings(got)
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 3)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	c.Assert(imgs[2].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0], imgs[2].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v1", "tsuru/app-appdeployimagetest:v2", "tsuru/python"}
	c.Assert(got, check.DeepEquals, expected)
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err = s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 3)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[1].RepoTags, check.HasLen, 1)
	c.Assert(imgs[2].RepoTags, check.HasLen, 1)
	got = []string{imgs[0].RepoTags[0], imgs[1].RepoTags[0], imgs[2].RepoTags[0]}
	sort.Strings(got)
	expected = []string{"tsuru/app-appdeployimagetest:v2", "tsuru/app-appdeployimagetest:v3", "tsuru/python"}
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUploadDeploy(c *check.C) {
	go s.stopContainers(3)
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	buf := bytes.NewBufferString("something wrong is not right")
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		File:         ioutil.NopCloser(buf),
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	units := a.Units()
	c.Assert(units, check.HasLen, 1)
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].Ip)
}

func (s *S) TestDeployRemoveContainersEvenWhenTheyreNotInTheAppsCollection(c *check.C) {
	go s.stopContainers(3)
	cont1, err := s.newContainer(nil, nil)
	defer s.removeTestContainer(cont1)
	c.Assert(err, check.IsNil)
	cont2, err := s.newContainer(nil, nil)
	defer s.removeTestContainer(cont2)
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(cont1.AppName)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	var w bytes.Buffer
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: &w,
	})
	c.Assert(err, check.IsNil)
	defer s.p.Destroy(&a)
	coll := s.p.collection()
	defer coll.Close()
	n, err := coll.Find(bson.M{"appname": cont1.AppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2)
}

func (s *S) TestImageDeploy(c *check.C) {
	go s.stopContainers(1)
	err := s.newFakeImage(s.p, "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
	})
	c.Assert(err, check.IsNil)
	units := a.Units()
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestImageDeployInvalidImage(c *check.C) {
	go s.stopContainers(1)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		OutputStream: w,
		Image:        "tsuru/app-otherapp:v1",
	})
	c.Assert(err, check.ErrorMatches, "invalid image for app otherapp: tsuru/app-otherapp:v1")
	units := a.Units()
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestImageDeployFailureDoesntEraseImage(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	s.server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		var result docker.Config
		err := json.Unmarshal(data, &result)
		if err == nil {
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
	})
	c.Assert(err, check.NotNil)
	units := a.Units()
	c.Assert(units, check.HasLen, 0)
	imgs, err := s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/app-otherapp:v1", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	unit := cont.asUnit(app)
	app.BindUnit(&unit)
	s.p.Provision(app)
	err = s.p.Destroy(app)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	count, err := coll.Find(bson.M{"appname": cont.AppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, false)
	c.Assert(app.HasBind(&unit), check.Equals, false)
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
	go s.stopContainers(1)
	a := app.App{
		Name:     "mydoomedapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	repository.Manager().CreateRepository(a.Name, nil)
	s.p.Provision(&a)
	defer s.p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(&a)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	count, err := coll.Find(bson.M{"appname": a.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, false)
	c.Assert(registryRequests, check.HasLen, 1)
	c.Assert(registryRequests[0].Method, check.Equals, "DELETE")
	c.Assert(registryRequests[0].URL.Path, check.Equals, "/v1/repositories/tsuru/app-mydoomedapp:v1/")
	imgs, err := s.p.getCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags[0], check.Equals, registryURL+"/tsuru/python")
}

func (s *S) TestProvisionerDestroyEmptyUnit(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(app)
	err := s.p.Destroy(app)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerDestroyRemovesRouterBackend(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(app)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(app)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, false)
}

func (s *S) TestProvisionerAddr(c *check.C) {
	cont, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	addr, err := s.p.Addr(app)
	c.Assert(err, check.IsNil)
	r, err := getRouterForApp(app)
	c.Assert(err, check.IsNil)
	expected, err := r.Addr(cont.AppName)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	app.Deploys = 1
	s.p.Provision(app)
	defer s.p.Destroy(app)
	_, err = s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.AddUnits(app, 3, nil)
	c.Assert(err, check.IsNil)
	coll := s.p.collection()
	defer coll.Close()
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	c.Assert(units, check.HasLen, 3)
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 4)
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
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(app)
	defer s.p.Destroy(app)
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	_, err = s.p.AddUnits(app, 3, nil)
	c.Assert(err, check.NotNil)
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestProvisionerAddZeroUnits(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	app.Deploys = 1
	s.p.Provision(app)
	defer s.p.Destroy(app)
	coll := s.p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	units, err := s.p.AddUnits(app, 0, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units")
}

func (s *S) TestProvisionerAddUnitsWithNoDeploys(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	s.p.Provision(app)
	defer s.p.Destroy(app)
	units, err := s.p.AddUnits(app, 1, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "New units can only be added after the first deployment")
}

func (s *S) TestProvisionerAddUnitsWithHost(c *check.C) {
	p, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(p)
	err = s.newFakeImage(p, "tsuru/app-myapp")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := p.collection()
	defer coll.Close()
	coll.Insert(container{ID: "xxxfoo", AppName: app.GetName(), Version: "123987", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "xxxfoo"})
	imageId, err := appCurrentImageName(app.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "localhost",
		unitsToAdd:  1,
		app:         app,
		imageId:     imageId,
		provisioner: p,
	})
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.GetName()})
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].HostAddr, check.Equals, "localhost")
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 2)
}

func (s *S) TestProvisionerRemoveUnits(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a1.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	err = s.p.RemoveUnits(papp, 2)
	c.Assert(err, check.IsNil)
	_, err = s.p.getContainer(cont1.ID)
	c.Assert(err, check.NotNil)
	_, err = s.p.getContainer(cont2.ID)
	c.Assert(err, check.NotNil)
	_, err = s.p.getContainer(cont3.ID)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerRemoveUnitsNotFound(c *check.C) {
	err := s.p.RemoveUnits(nil, 1)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: app should not be nil")
}

func (s *S) TestProvisionerRemoveUnitsZeroUnits(c *check.C) {
	err := s.p.RemoveUnits(provisiontest.NewFakeApp("something", "python", 0), 0)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: units must be at least 1")
}

func (s *S) TestProvisionerRemoveUnitsTooManyUnits(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a1.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.scheduler = &scheduler
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	papp := provisiontest.NewFakeApp(a1.Name, "python", 0)
	s.p.Provision(papp)
	err = s.p.RemoveUnits(papp, 4)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: cannot remove 4 units. App impius has just 3 units.")
}

func (s *S) TestProvisionerRemoveUnit(c *check.C) {
	container, err := s.newContainer(nil, nil)
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(container.AppName)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	a := app.App{Name: container.AppName, Platform: "python"}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = client.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveUnit(provision.Unit{AppName: a.Name, Name: container.ID})
	c.Assert(err, check.IsNil)
	_, err = s.p.getContainer(container.ID)
	c.Assert(err, check.NotNil)
	c.Assert(routertest.FakeRouter.HasRoute(container.AppName, container.getAddress()), check.Equals, false)
}

func (s *S) TestProvisionerRemoveUnitNotFound(c *check.C) {
	err := s.p.RemoveUnit(provision.Unit{Name: "wat de reu"})
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestProvisionerSetUnitStatus(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName}, provision.StatusError)
	c.Assert(err, check.IsNil)
	container, err = s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusWrongApp(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = s.p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName + "a"}, provision.StatusError)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "wrong app name")
	container, err = s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestProvisionerSetUnitStatusUnitNotFound(c *check.C) {
	err := s.p.SetUnitStatus(provision.Unit{Name: "mycontainer", AppName: "myapp"}, provision.StatusError)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestProvisionerExecuteCommand(c *check.C) {
	app := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container1)
	coll := s.p.collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container1.ID}, container1)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	coll.Update(bson.M{"id": container2.ID}, container2)
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(&stdout, &stderr, app, "ls", "-l")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := s.p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionerExecuteCommandExcludesBuildContainers(c *check.C) {
	app := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container3, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container4, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	container2.setStatus(s.p, provision.StatusCreated.String())
	container3.setStatus(s.p, provision.StatusBuilding.String())
	container4.setStatus(s.p, provision.StatusStopped.String())
	containers := []*container{
		container1,
		container2,
		container3,
		container4,
	}
	coll := s.p.collection()
	defer coll.Close()
	for _, c := range containers {
		defer s.removeTestContainer(c)
	}
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(&stdout, &stderr, app, "echo x")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandOnce(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	coll := s.p.collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommandOnce(&stdout, &stderr, app, "ls", "-l")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandOnceNoContainers(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := s.p.ExecuteCommandOnce(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionCollection(c *check.C) {
	collection := s.p.collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, s.collName)
}

func (s *S) TestProvisionSetCName(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	routertest.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := s.p.SetCName(app, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, "127.0.0.1"), check.Equals, true)
}

func (s *S) TestProvisionUnsetCName(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	routertest.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := s.p.SetCName(app, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, "127.0.0.1"), check.Equals, true)
	err = s.p.UnsetCName(app, cname)
	c.Assert(routertest.FakeRouter.HasBackend(cname), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(cname, "127.0.0.1"), check.Equals, false)
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
		fixContainersCmd{},
		&listHealingHistoryCmd{},
		&listAutoScaleHistoryCmd{},
		&updateNodeToSchedulerCmd{},
		&listAutoScaleRunCmd{},
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
	routertest.FakeRouter.AddRoute(app1.GetName(), "127.0.0.1")
	routertest.FakeRouter.AddBackend(app2.GetName())
	routertest.FakeRouter.AddRoute(app2.GetName(), "127.0.0.2")
	err := s.p.Swap(app1, app2)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app1.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasBackend(app2.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app2.GetName(), "127.0.0.1"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app1.GetName(), "127.0.0.2"), check.Equals, true)
}

func (s *S) TestProvisionerStart(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(&app.App{Name: "almah"})
	c.Assert(err, check.IsNil)
	defer conn.Apps().RemoveAll(bson.M{"name": "almah"})
	app := provisiontest.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	err = s.p.Start(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	container, err = s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	expectedIP := dockerContainer.NetworkSettings.IPAddress
	expectedPort := dockerContainer.NetworkSettings.Ports["8888/tcp"][0].HostPort
	c.Assert(container.IP, check.Equals, expectedIP)
	c.Assert(container.HostPort, check.Equals, expectedPort)
	c.Assert(container.Status, check.Equals, provision.StatusStarting.String())
}

func (s *S) TestProvisionerStop(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	app := provisiontest.NewFakeApp("almah", "static", 2)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = s.p.Stop(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerStopSkipAlreadyStoppedContainers(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	app := provisiontest.NewFakeApp("almah", "static", 2)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	err = dcli.StartContainer(container2.ID, nil)
	c.Assert(err, check.IsNil)
	err = dcli.StopContainer(container2.ID, 1)
	c.Assert(err, check.IsNil)
	dockerContainer2, err := dcli.InspectContainer(container2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer2.State.Running, check.Equals, false)
	err = s.p.Stop(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	dockerContainer2, err = dcli.InspectContainer(container2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer2.State.Running, check.Equals, false)
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
	err = p.PlatformAdd("test", args, bytes.NewBuffer(nil))
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].URL.Path, check.Equals, "/build")
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), check.Equals, platformImageName("test"))
	c.Assert(queryString.Get("remote"), check.Equals, "http://localhost/Dockerfile")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test/json")
	c.Assert(requests[2].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test/push")
}

func (s *S) TestProvisionerPlatformAddWithoutArgs(c *check.C) {
	err := s.p.PlatformAdd("test", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Dockerfile is required.")
}

func (s *S) TestProvisionerPlatformAddShouldValidateArgs(c *check.C) {
	args := make(map[string]string)
	args["dockerfile"] = "not_a_url"
	err := s.p.PlatformAdd("test", args, bytes.NewBuffer(nil))
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "dockerfile parameter should be an url.")
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
	err = p.PlatformAdd("test", args, bytes.NewBuffer(nil))
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerPlatformRemove(c *check.C) {
	registryServer := httptest.NewServer(nil)
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
	err = p.PlatformAdd("test", map[string]string{"dockerfile": "http://localhost/Dockerfile"}, &buf)
	c.Assert(err, check.IsNil)
	err = p.PlatformRemove("test")
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 4)
	c.Assert(requests[3].Method, check.Equals, "DELETE")
	c.Assert(requests[3].URL.Path, check.Matches, "/images/[^/]+")
}

func (s *S) TestProvisionerPlatformRemoveReturnsStorageError(c *check.C) {
	registryServer := httptest.NewServer(nil)
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
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	units := s.p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsAppDoesNotExist(c *check.C) {
	app := app.App{Name: "myapplication"}
	units := s.p.Units(&app)
	expected := []provision.Unit{}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsStatus(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(
		container{
			ID:       "9930c24f1c4f",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusBuilding.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
		container{
			ID:       "9930c24f1c4j",
			AppName:  app.Name,
			Type:     "python",
			Status:   provision.StatusError.String(),
			IP:       "127.0.0.4",
			HostPort: "9025",
		},
	)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"appname": app.Name})
	units := s.p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
		{Name: "9930c24f1c4j", AppName: "myapplication", Type: "python", Status: provision.StatusError},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsIp(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := s.p.collection()
	defer coll.Close()
	err := coll.Insert(
		container{
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
	units := s.p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Ip: "127.0.0.1", Status: provision.StatusBuilding},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestRegisterUnit(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(&app.App{Name: "myawesomeapp"})
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarting.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.RegisterUnit(provision.Unit{Name: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `\d+\.\d+\.\d+\.\d+`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestRegisterUnitBuildingContainer(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	err = s.p.RegisterUnit(provision.Unit{Name: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := s.p.getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `xinvalidx`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusBuilding.String())
}

func (s *S) TestRegisterUnitSavesCustomData(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/python")
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"mydata": "value"}
	err = s.p.RegisterUnit(provision.Unit{Name: container.ID}, data)
	c.Assert(err, check.IsNil)
	dataColl, err := imageCustomDataColl()
	c.Assert(err, check.IsNil)
	var customData map[string]interface{}
	err = dataColl.FindId(container.BuildingImage).One(&customData)
	c.Assert(err, check.IsNil)
	c.Assert(customData["customdata"], check.DeepEquals, data)
}

func (s *S) TestRunRestartAfterHooks(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	a := &app.App{
		Name: "myrestartafterapp",
		CustomData: map[string]interface{}{
			"hooks": map[string]interface{}{
				"restart": map[string]interface{}{
					"after": []string{"cmd1", "cmd2"},
				},
			},
		},
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
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
	defer container.remove(s.p)
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
	err := s.newFakeImage(s.p, "tsuru/app-almah")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ShellOptions{App: app, Conn: conn, Width: 10, Height: 10, Unit: cont.ID}
	err = s.p.Shell(opts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShellToAnAppByAppName(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-almah")
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()}, nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ShellOptions{App: app, Conn: conn, Width: 10, Height: 10}
	err = s.p.Shell(opts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDryMode(c *check.C) {
	err := s.newFakeImage(s.p, "tsuru/app-myapp")
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer s.p.Destroy(appInstance)
	s.p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		unitsToAdd:  5,
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
