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
	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository/repositorytest"
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
	var p dockerProvisioner
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, true)
	c.Assert(app.IsReady(), check.Equals, true)
}

func (s *S) TestProvisionerRestart(c *check.C) {
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	err = p.Start(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dCluster.InspectContainer(cont.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = p.Restart(app, nil)
	c.Assert(err, check.IsNil)
	dbConts, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 1)
	c.Assert(dbConts[0].ID, check.Not(check.Equals), cont.ID)
	c.Assert(dbConts[0].AppName, check.Equals, app.GetName())
	c.Assert(dbConts[0].Status, check.Equals, provision.StatusStarting.String())
	dockerContainer, err = dCluster.InspectContainer(dbConts[0].ID)
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(3)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := dockerCluster().ListImages(docker.ListImagesOptions{All: true})
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
	imgs, err = dockerCluster().ListImages(docker.ListImagesOptions{All: true})
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
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer p.Destroy(&a)
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
	imgs, err := dockerCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/python", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestDeployErasesOldImagesWithLongHistory(c *check.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(5)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "appdeployimagetest",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = p.Provision(&a)
	c.Assert(err, check.IsNil)
	defer p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	imgs, err := dockerCluster().ListImages(docker.ListImagesOptions{All: true})
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
	imgs, err = dockerCluster().ListImages(docker.ListImagesOptions{All: true})
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
	imgs, err = dockerCluster().ListImages(docker.ListImagesOptions{All: true})
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(3)
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(3)
	cont1, err := s.newContainer(nil)
	defer s.removeTestContainer(cont1)
	c.Assert(err, check.IsNil)
	cont2, err := s.newContainer(nil)
	defer s.removeTestContainer(cont2)
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(cont1.AppName)
	var p dockerProvisioner
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	var w bytes.Buffer
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: &w,
	})
	c.Assert(err, check.IsNil)
	defer p.Destroy(&a)
	coll := collection()
	defer coll.Close()
	n, err := coll.Find(bson.M{"appname": cont1.AppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2)
}

func (s *S) TestImageDeploy(c *check.C) {
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	err := newImage("tsuru/app-otherapp:v1", s.server.URL())
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
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
	err := newImage("tsuru/app-otherapp:v1", s.server.URL())
	c.Assert(err, check.IsNil)
	err = appendAppImageName("otherapp", "tsuru/app-otherapp:v1")
	c.Assert(err, check.IsNil)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "otherapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
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
	imgs, err := dockerCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert("tsuru/app-otherapp:v1", check.Equals, imgs[0].RepoTags[0])
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	cont, err := s.newContainer(nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	unit := cont.asUnit(app)
	app.BindUnit(&unit)
	var p dockerProvisioner
	p.Provision(app)
	err = p.Destroy(app)
	c.Assert(err, check.IsNil)
	coll := collection()
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
	h := &apitest.TestHandler{}
	gandalfServer := repositorytest.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	go s.stopContainers(1)
	p := dockerProvisioner{}
	a := app.App{
		Name:     "mydoomedapp",
		Platform: "python",
	}
	conn, err := db.Conn()
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	p.Provision(&a)
	defer p.Destroy(&a)
	w := safe.NewBuffer(make([]byte, 2048))
	err = app.Deploy(app.DeployOptions{
		App:          &a,
		Version:      "master",
		Commit:       "123",
		OutputStream: w,
	})
	c.Assert(err, check.IsNil)
	err = p.Destroy(&a)
	c.Assert(err, check.IsNil)
	coll := collection()
	count, err := coll.Find(bson.M{"appname": a.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, false)
	c.Assert(registryRequests, check.HasLen, 1)
	c.Assert(registryRequests[0].Method, check.Equals, "DELETE")
	c.Assert(registryRequests[0].URL.Path, check.Equals, "/v1/repositories/tsuru/app-mydoomedapp:v1/")
	imgs, err := dockerCluster().ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(imgs, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags, check.HasLen, 1)
	c.Assert(imgs[0].RepoTags[0], check.Equals, registryURL+"/tsuru/python")
}

func (s *S) TestProvisionerDestroyEmptyUnit(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	p.Provision(app)
	err := p.Destroy(app)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerDestroyRemovesRouterBackend(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	err = p.Destroy(app)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend("myapp"), check.Equals, false)
}

func (s *S) TestProvisionerAddr(c *check.C) {
	cont, err := s.newContainer(nil)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	app := provisiontest.NewFakeApp(cont.AppName, "python", 1)
	var p dockerProvisioner
	addr, err := p.Addr(app)
	c.Assert(err, check.IsNil)
	r, err := getRouterForApp(app)
	c.Assert(err, check.IsNil)
	expected, err := r.Addr(cont.AppName)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, expected)
}

func (s *S) TestProvisionerAddUnits(c *check.C) {
	err := newImage("tsuru/app-myapp", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	_, err = s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	units, err := p.AddUnits(app, 3, nil)
	c.Assert(err, check.IsNil)
	coll := collection()
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
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	_, err = p.AddUnits(app, 3, nil)
	c.Assert(err, check.NotNil)
	count, err := coll.Find(bson.M{"appname": app.GetName()}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestProvisionerAddZeroUnits(c *check.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "c-89320", AppName: app.GetName(), Version: "a345fe", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "c-89320"})
	units, err := p.AddUnits(app, 0, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units")
}

func (s *S) TestProvisionerAddUnitsWithoutContainers(c *check.C) {
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	var p dockerProvisioner
	p.Provision(app)
	defer p.Destroy(app)
	units, err := p.AddUnits(app, 1, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "New units can only be added after the first deployment")
}

func (s *S) TestProvisionerAddUnitsWithHost(c *check.C) {
	cluster, err := s.startMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer s.stopMultipleServersCluster(cluster)
	err = newImage("tsuru/app-myapp", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "xxxfoo", AppName: app.GetName(), Version: "123987", Image: "tsuru/python"})
	defer coll.RemoveId(bson.M{"id": "xxxfoo"})
	imageId, err := appCurrentImageName(app.GetName())
	c.Assert(err, check.IsNil)
	units, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "localhost",
		unitsToAdd: 1,
		app:        app,
		imageId:    imageId,
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
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	container1, err := s.newContainer(&newContainerOpts{Status: "building"})
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(container1.AppName)
	container2, err := s.newContainer(&newContainerOpts{Status: "building"})
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(container2.AppName)
	container3, err := s.newContainer(&newContainerOpts{Status: "started"})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container3)
	client, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	err = client.StartContainer(container1.ID, nil)
	c.Assert(err, check.IsNil)
	err = client.StartContainer(container2.ID, nil)
	c.Assert(err, check.IsNil)
	app := provisiontest.NewFakeApp(container1.AppName, "python", 0)
	unit1 := container1.asUnit(app)
	unit2 := container2.asUnit(app)
	unit3 := container3.asUnit(app)
	app.BindUnit(&unit1)
	app.BindUnit(&unit2)
	app.BindUnit(&unit3)
	var p dockerProvisioner
	err = p.RemoveUnits(app, 2)
	c.Assert(err, check.IsNil)
	_, err = getContainer(container1.ID)
	c.Assert(err, check.NotNil)
	_, err = getContainer(container2.ID)
	c.Assert(err, check.NotNil)
	c.Check(app.HasBind(&unit1), check.Equals, false)
	c.Check(app.HasBind(&unit2), check.Equals, false)
	c.Check(app.HasBind(&unit3), check.Equals, true)
}

func (s *S) TestProvisionerRemoveUnitsPriorityOrder(c *check.C) {
	container, err := s.newContainer(nil)
	c.Assert(err, check.IsNil)
	err = newImage("tsuru/app-"+container.AppName, "")
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(container.AppName)
	app := provisiontest.NewFakeApp(container.AppName, "python", 0)
	var p dockerProvisioner
	_, err = p.AddUnits(app, 3, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(app, 1)
	c.Assert(err, check.IsNil)
	_, err = getContainer(container.ID)
	c.Assert(err, check.NotNil)
	c.Assert(p.Units(app), check.HasLen, 3)
}

func (s *S) TestProvisionerRemoveUnitsNotFound(c *check.C) {
	var p dockerProvisioner
	err := p.RemoveUnits(nil, 1)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: app should not be nil")
}

func (s *S) TestProvisionerRemoveUnitsZeroUnits(c *check.C) {
	var p dockerProvisioner
	err := p.RemoveUnits(provisiontest.NewFakeApp("something", "python", 0), 0)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: units must be at least 1")
}

func (s *S) TestProvisionerRemoveUnitsTooManyUnits(c *check.C) {
	container, err := s.newContainer(nil)
	c.Assert(err, check.IsNil)
	err = newImage("tsuru/app-"+container.AppName, "")
	c.Assert(err, check.IsNil)
	defer routertest.FakeRouter.RemoveBackend(container.AppName)
	app := provisiontest.NewFakeApp(container.AppName, "python", 0)
	var p dockerProvisioner
	_, err = p.AddUnits(app, 2, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "remove units: cannot remove all units from app")
}

func (s *S) TestProvisionerRemoveUnit(c *check.C) {
	container, err := s.newContainer(nil)
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
	var p dockerProvisioner
	err = p.RemoveUnit(provision.Unit{AppName: a.Name, Name: container.ID})
	c.Assert(err, check.IsNil)
	_, err = getContainer(container.ID)
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionerRemoveUnitNotFound(c *check.C) {
	var p dockerProvisioner
	err := p.RemoveUnit(provision.Unit{Name: "wat de reu"})
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestProvisionerSetUnitStatus(c *check.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	var p dockerProvisioner
	err = p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName}, provision.StatusError)
	c.Assert(err, check.IsNil)
	container, err = getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusError.String())
}

func (s *S) TestProvisionerSetUnitStatusWrongApp(c *check.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarted.String(), AppName: "someapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	var p dockerProvisioner
	err = p.SetUnitStatus(provision.Unit{Name: container.ID, AppName: container.AppName + "a"}, provision.StatusError)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "wrong app name")
	container, err = getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestProvisionerSetUnitStatusUnitNotFound(c *check.C) {
	var p dockerProvisioner
	err := p.SetUnitStatus(provision.Unit{Name: "mycontainer", AppName: "myapp"}, provision.StatusError)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestProvisionerExecuteCommand(c *check.C) {
	app := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container1)
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container1.ID}, container1)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	coll.Update(bson.M{"id": container2.ID}, container2)
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "ls", "-l")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandNoContainers(c *check.C) {
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionerExecuteCommandExcludesBuildContainers(c *check.C) {
	app := provisiontest.NewFakeApp("starbreaker", "python", 1)
	container1, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	container3, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	container4, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	container2.setStatus(provision.StatusCreated.String())
	container3.setStatus(provision.StatusBuilding.String())
	container4.setStatus(provision.StatusStopped.String())
	containers := []*container{
		container1,
		container2,
		container3,
		container4,
	}
	coll := collection()
	defer coll.Close()
	for _, c := range containers {
		defer s.removeTestContainer(c)
	}
	var stdout, stderr bytes.Buffer
	var p dockerProvisioner
	err = p.ExecuteCommand(&stdout, &stderr, app, "echo x")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandOnce(c *check.C) {
	app := provisiontest.NewFakeApp("almah", "static", 1)
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	coll := collection()
	defer coll.Close()
	coll.Update(bson.M{"id": container.ID}, container)
	var stdout, stderr bytes.Buffer
	err = p.ExecuteCommandOnce(&stdout, &stderr, app, "ls", "-l")
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerExecuteCommandOnceNoContainers(c *check.C) {
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("almah", "static", 2)
	var buf bytes.Buffer
	err := p.ExecuteCommandOnce(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, check.Equals, provision.ErrEmptyApp)
}

func (s *S) TestProvisionCollection(c *check.C) {
	collection := collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, s.collName)
}

func (s *S) TestProvisionSetCName(c *check.C) {
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	routertest.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := p.SetCName(app, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, "127.0.0.1"), check.Equals, true)
}

func (s *S) TestProvisionUnsetCName(c *check.C) {
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("myapp", "python", 1)
	routertest.FakeRouter.AddBackend("myapp")
	routertest.FakeRouter.AddRoute("myapp", "127.0.0.1")
	cname := "mycname.com"
	err := p.SetCName(app, cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(cname), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(cname, "127.0.0.1"), check.Equals, true)
	err = p.UnsetCName(app, cname)
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
		addPoolToSchedulerCmd{},
		&removePoolFromSchedulerCmd{},
		listPoolsInTheSchedulerCmd{},
		addTeamsToPoolCmd{},
		removeTeamsFromPoolCmd{},
		fixContainersCmd{},
		&listHealingHistoryCmd{},
	}
	var p dockerProvisioner
	c.Assert(p.AdminCommands(), check.DeepEquals, expected)
}

func (s *S) TestProvisionerIsAdminCommandable(c *check.C) {
	var _ cmd.AdminCommandable = &dockerProvisioner{}
}

func (s *S) TestSwap(c *check.C) {
	var p dockerProvisioner
	app1 := provisiontest.NewFakeApp("app1", "python", 1)
	app2 := provisiontest.NewFakeApp("app2", "python", 1)
	routertest.FakeRouter.AddBackend(app1.GetName())
	routertest.FakeRouter.AddRoute(app1.GetName(), "127.0.0.1")
	routertest.FakeRouter.AddBackend(app2.GetName())
	routertest.FakeRouter.AddRoute(app2.GetName(), "127.0.0.2")
	err := p.Swap(app1, app2)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app1.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasBackend(app2.GetName()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app2.GetName(), "127.0.0.1"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(app1.GetName(), "127.0.0.2"), check.Equals, true)
}

func (s *S) TestProvisionerStart(c *check.C) {
	var p dockerProvisioner
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(&app.App{Name: "almah"})
	c.Assert(err, check.IsNil)
	defer conn.Apps().RemoveAll(bson.M{"name": "almah"})
	app := provisiontest.NewFakeApp("almah", "static", 1)
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	dcli, err := docker.NewClient(s.server.URL())
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
	err = p.Start(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	container, err = getContainer(container.ID)
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
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	err = p.Stop(app)
	c.Assert(err, check.IsNil)
	dockerContainer, err = dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, false)
}

func (s *S) TestProvisionerStopSkipAlreadyStoppedContainers(c *check.C) {
	dcli, _ := docker.NewClient(s.server.URL())
	app := provisiontest.NewFakeApp("almah", "static", 2)
	p := dockerProvisioner{}
	container, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	err = dcli.StartContainer(container.ID, nil)
	c.Assert(err, check.IsNil)
	dockerContainer, err := dcli.InspectContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer.State.Running, check.Equals, true)
	container2, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container2)
	err = dcli.StartContainer(container2.ID, nil)
	c.Assert(err, check.IsNil)
	err = dcli.StopContainer(container2.ID, 1)
	c.Assert(err, check.IsNil)
	dockerContainer2, err := dcli.InspectContainer(container2.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dockerContainer2.State.Running, check.Equals, false)
	err = p.Stop(app)
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
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	p := dockerProvisioner{}
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
	p := dockerProvisioner{}
	err := p.PlatformAdd("test", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Dockerfile is required.")
}

func (s *S) TestProvisionerPlatformAddShouldValidateArgs(c *check.C) {
	args := make(map[string]string)
	args["dockerfile"] = "not_a_url"
	p := dockerProvisioner{}
	err := p.PlatformAdd("test", args, bytes.NewBuffer(nil))
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
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	p := dockerProvisioner{}
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
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	p := dockerProvisioner{}
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
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &strg,
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		dCluster = oldDockerCluster
		cmutex.Unlock()
	}()
	p := dockerProvisioner{}
	err = p.PlatformRemove("test")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.DeepEquals, storage.ErrNoSuchImage)
}

func (s *S) TestProvisionerUnits(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
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
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsAppDoesNotExist(c *check.C) {
	app := app.App{Name: "myapplication"}
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsStatus(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
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
	p := dockerProvisioner{}
	units := p.Units(&app)
	expected := []provision.Unit{
		{Name: "9930c24f1c4f", AppName: "myapplication", Type: "python", Status: provision.StatusBuilding},
		{Name: "9930c24f1c4j", AppName: "myapplication", Type: "python", Status: provision.StatusError},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestProvisionerUnitsIp(c *check.C) {
	app := app.App{Name: "myapplication"}
	coll := collection()
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
	p := dockerProvisioner{}
	units := p.Units(&app)
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
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusStarting.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.RegisterUnit(provision.Unit{Name: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `\d+\.\d+\.\d+\.\d+`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusStarted.String())
}

func (s *S) TestRegisterUnitBuildingContainer(c *check.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	coll := collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.RegisterUnit(provision.Unit{Name: container.ID}, nil)
	c.Assert(err, check.IsNil)
	dbCont, err := getContainer(container.ID)
	c.Assert(err, check.IsNil)
	c.Assert(dbCont.IP, check.Matches, `xinvalidx`)
	c.Assert(dbCont.Status, check.Equals, provision.StatusBuilding.String())
}

func (s *S) TestRegisterUnitSavesCustomData(c *check.C) {
	err := newImage("tsuru/python", s.server.URL())
	c.Assert(err, check.IsNil)
	opts := newContainerOpts{Status: provision.StatusBuilding.String(), AppName: "myawesomeapp"}
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	container.IP = "xinvalidx"
	container.BuildingImage = "my-building-image"
	coll := collection()
	defer coll.Close()
	err = coll.Update(bson.M{"id": container.ID}, container)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	data := map[string]interface{}{"mydata": "value"}
	err = p.RegisterUnit(provision.Unit{Name: container.ID}, data)
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
	container, err := s.newContainer(&opts)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(container)
	var reqBodies [][]byte
	s.server.CustomHandler("/containers/"+container.ID+"/exec", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		reqBodies = append(reqBodies, data)
		s.server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer container.remove()
	var buf bytes.Buffer
	err = runRestartAfterHooks(container, &buf)
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

func (s *S) TestAddContainersWithHostFailsUnlessRestartAfter(c *check.C) {
	s.server.CustomHandler("/exec/id-exec-created-by-test/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ID": "id-exec-created-by-test", "ExitCode": 9}`))
	}))
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	a := &app.App{
		Name: "myapp",
		CustomData: map[string]interface{}{
			"hooks": map[string]interface{}{
				"restart": map[string]interface{}{
					"after": []string{"will fail"},
				},
			},
		},
	}
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = newImage("tsuru/app-"+a.Name, s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp(a.Name, "python", 0)
	p.Provision(app)
	defer p.Destroy(app)
	var buf bytes.Buffer
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		unitsToAdd: 1,
		app:        app,
		writer:     &buf,
		imageId:    "tsuru/app-" + a.Name,
	})
	c.Assert(err, check.ErrorMatches, `couldn't execute restart:after hook "will fail"\(.+?\): unexpected exit code: 9`)
}

func (s *S) TestShellToAnAppByContainerID(c *check.C) {
	err := newImage("tsuru/app-almah", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{buf}
	err = p.Shell(app, conn, 10, 10, cont.ID)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShellToAnAppByAppName(c *check.C) {
	err := newImage("tsuru/app-almah", s.server.URL())
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	app := provisiontest.NewFakeApp("almah", "static", 1)
	cont, err := s.newContainer(&newContainerOpts{AppName: app.GetName()})
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(cont)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{buf}
	err = p.Shell(app, conn, 10, 10, "")
	c.Assert(err, check.IsNil)
}
