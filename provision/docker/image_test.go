// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"sort"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestMigrateImages(c *check.C) {
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
	app1 := app.App{Name: "app1"}
	app2 := app.App{Name: "app2"}
	app3 := app.App{Name: "app-app2"}
	c1, err := s.newContainer(&newContainerOpts{Image: "tsuru/app1", AppName: "app1"}, &p)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(c1)
	c2, err := s.newContainer(&newContainerOpts{Image: "tsuru/app1", AppName: "app1"}, &p)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(c2)
	c3, err := s.newContainer(&newContainerOpts{Image: "tsuru/app1", AppName: "app1"}, &p)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(c3)
	c4, err := s.newContainer(&newContainerOpts{Image: "tsuru/app-app2", AppName: "app2"}, &p)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(c4)
	c5, err := s.newContainer(&newContainerOpts{Image: "tsuru/app-app2", AppName: "app-app2"}, &p)
	c.Assert(err, check.IsNil)
	defer s.removeTestContainer(c5)
	err = s.storage.Apps().Insert(app1, app2, app3)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	err = MigrateImages()
	c.Assert(err, check.IsNil)
	contApp1, err := p.ListContainers(bson.M{"appname": app1.Name, "image": "tsuru/app-app1"})
	c.Assert(err, check.IsNil)
	c.Assert(contApp1, check.HasLen, 3)
	contApp2, err := p.ListContainers(bson.M{"appname": app2.Name, "image": "tsuru/app-app-app2"})
	c.Assert(err, check.IsNil)
	c.Assert(contApp2, check.HasLen, 0)
	contApp3, err := p.ListContainers(bson.M{"appname": app3.Name, "image": "tsuru/app-app-app2"})
	c.Assert(err, check.IsNil)
	c.Assert(contApp3, check.HasLen, 1)
}

func (s *S) TestMigrateImagesWithoutImageInStorage(c *check.C) {
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
	app1 := app.App{Name: "app1"}
	err = s.storage.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	err = MigrateImages()
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(images, check.HasLen, 0)
}

func (s *S) TestMigrateImagesWithRegistry(c *check.C) {
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
	app1 := app.App{Name: "app1"}
	app2 := app.App{Name: "app2"}
	err = s.storage.Apps().Insert(app1, app2)
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(&p, "localhost:3030/tsuru/app1", nil)
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(&p, "localhost:3030/tsuru/app2", nil)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	err = MigrateImages()
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(images, check.HasLen, 2)
	tags1 := images[0].RepoTags
	sort.Strings(tags1)
	tags2 := images[1].RepoTags
	sort.Strings(tags2)
	c.Assert(tags1, check.DeepEquals, []string{"localhost:3030/tsuru/app-app1", "localhost:3030/tsuru/app1"})
	c.Assert(tags2, check.DeepEquals, []string{"localhost:3030/tsuru/app-app2", "localhost:3030/tsuru/app2"})
}

func (s *S) TestUsePlatformImage(c *check.C) {
	app1 := &app.App{Name: "app1", Platform: "python", Deploys: 40}
	err := s.storage.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	ok := s.p.usePlatformImage(app1)
	c.Assert(ok, check.Equals, true)
	app2 := &app.App{Name: "app2", Platform: "python", Deploys: 20}
	err = s.storage.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app2)
	c.Assert(ok, check.Equals, true)
	app3 := &app.App{Name: "app3", Platform: "python", Deploys: 0}
	err = s.storage.Apps().Insert(app3)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app3)
	c.Assert(ok, check.Equals, true)
	app4 := &app.App{Name: "app4", Platform: "python", Deploys: 19}
	err = s.storage.Apps().Insert(app4)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app4)
	c.Assert(ok, check.Equals, false)
	app5 := &app.App{
		Name:           "app5",
		Platform:       "python",
		Deploys:        19,
		UpdatePlatform: true,
	}
	err = s.storage.Apps().Insert(app5)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app5)
	c.Assert(ok, check.Equals, true)
	app6 := &app.App{Name: "app6", Platform: "python", Deploys: 19}
	err = s.storage.Apps().Insert(app6)
	c.Assert(err, check.IsNil)
	coll := s.p.Collection()
	defer coll.Close()
	err = coll.Insert(container.Container{AppName: app6.Name, Image: "tsuru/app-app6"})
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app6)
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestAppNewImageName(c *check.C) {
	img1, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
	img2, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
	img3, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "tsuru/app-myapp:v3")
}

func (s *S) TestAppNewImageNameWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	img1, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/app-myapp:v1")
	img2, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "localhost:3030/tsuru/app-myapp:v2")
	img3, err := appNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "localhost:3030/tsuru/app-myapp:v3")
}

func (s *S) TestAppCurrentImageNameWithoutImage(c *check.C) {
	img1, err := appCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp")
}

func (s *S) TestAppendAppImageChangeImagePosition(c *check.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	images, err := listAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v1"})
}

func (s *S) TestAppCurrentImageName(c *check.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	img1, err := appCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	img2, err := appCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
}

func (s *S) TestListAppImages(c *check.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	images, err := listAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:v2"})
}

func (s *S) TestValidListAppImages(c *check.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	images, err := listValidAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v3"})
}

func (s *S) TestPlatformImageName(c *check.C) {
	platName := platformImageName("python")
	c.Assert(platName, check.Equals, "tsuru/python:latest")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	platName = platformImageName("ruby")
	c.Assert(platName, check.Equals, "localhost:3030/tsuru/ruby:latest")
}

func (s *S) TestDeleteAllAppImageNames(c *check.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = deleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = listAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
}

func (s *S) TestDeleteAllAppImageNamesRemovesCustomData(c *check.C) {
	imgName := "tsuru/app-myapp:v1"
	err := appendAppImageName("myapp", imgName)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err = saveImageCustomData(imgName, data)
	c.Assert(err, check.IsNil)
	err = deleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = listAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	yamlData, err := getImageTsuruYamlData(imgName)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestDeleteAllAppImageNamesRemovesCustomDataWithoutImages(c *check.C) {
	imgName := "tsuru/app-myapp:v1"
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err := saveImageCustomData(imgName, data)
	c.Assert(err, check.IsNil)
	err = deleteAllAppImageNames("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	yamlData, err := getImageTsuruYamlData(imgName)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestDeleteAllAppImageNamesSimilarApps(c *check.C) {
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = saveImageCustomData("tsuru/app-myapp:v1", data)
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp-dev", "tsuru/app-myapp-dev:v1")
	c.Assert(err, check.IsNil)
	err = saveImageCustomData("tsuru/app-myapp-dev:v1", data)
	c.Assert(err, check.IsNil)
	err = deleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = listAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	_, err = listAppImages("myapp-dev")
	c.Assert(err, check.IsNil)
	yamlData, err := getImageTsuruYamlData("tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
	yamlData, err = getImageTsuruYamlData("tsuru/app-myapp-dev:v1")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{
		Healthcheck: provision.TsuruYamlHealthcheck{Path: "/test"},
	})
}

func (s *S) TestPullAppImageNames(c *check.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	err = pullAppImageNames("myapp", []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:v3"})
	c.Assert(err, check.IsNil)
	images, err := listAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2"})
}

func (s *S) TestPullAppImageNamesRemovesCustomData(c *check.C) {
	img1Name := "tsuru/app-myapp:v1"
	err := appendAppImageName("myapp", img1Name)
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err = saveImageCustomData(img1Name, data)
	c.Assert(err, check.IsNil)
	err = pullAppImageNames("myapp", []string{img1Name})
	c.Assert(err, check.IsNil)
	images, err := listAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v3"})
	yamlData, err := getImageTsuruYamlData(img1Name)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestGetImageWebProcessName(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "someworker",
		},
	}
	err := saveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	img2 := "tsuru/app-myapp:v2"
	customData2 := map[string]interface{}{
		"processes": map[string]interface{}{
			"worker1": "python myapp.py",
			"worker2": "someworker",
		},
	}
	err = saveImageCustomData(img2, customData2)
	c.Assert(err, check.IsNil)
	img3 := "tsuru/app-myapp:v3"
	customData3 := map[string]interface{}{
		"processes": map[string]interface{}{
			"api": "python myapi.py",
		},
	}
	err = saveImageCustomData(img3, customData3)
	c.Assert(err, check.IsNil)
	img4 := "tsuru/app-myapp:v4"
	customData4 := map[string]interface{}{}
	err = saveImageCustomData(img4, customData4)
	c.Assert(err, check.IsNil)
	web1, err := getImageWebProcessName(img1)
	c.Check(err, check.IsNil)
	c.Check(web1, check.Equals, "web")
	web2, err := getImageWebProcessName(img2)
	c.Check(err, check.IsNil)
	c.Check(web2, check.Equals, "web")
	web3, err := getImageWebProcessName(img3)
	c.Check(err, check.IsNil)
	c.Check(web3, check.Equals, "api")
	web4, err := getImageWebProcessName(img4)
	c.Check(err, check.IsNil)
	c.Check(web4, check.Equals, "")
	img5 := "tsuru/app-myapp:v5"
	web5, err := getImageWebProcessName(img5)
	c.Check(err, check.IsNil)
	c.Check(web5, check.Equals, "")
}
