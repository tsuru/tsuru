// Copyright 2015 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1, app2, app3)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name, app2.Name, app3.Name}}})
	mainDockerProvisioner = &p
	err = MigrateImages()
	c.Assert(err, check.IsNil)
	contApp1, err := p.listContainersBy(bson.M{"appname": app1.Name, "image": "tsuru/app-app1"})
	c.Assert(err, check.IsNil)
	c.Assert(contApp1, check.HasLen, 3)
	contApp2, err := p.listContainersBy(bson.M{"appname": app2.Name, "image": "tsuru/app-app-app2"})
	c.Assert(err, check.IsNil)
	c.Assert(contApp2, check.HasLen, 0)
	contApp3, err := p.listContainersBy(bson.M{"appname": app3.Name, "image": "tsuru/app-app-app2"})
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name}}})
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1, app2)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name, app2.Name}}})
	err = s.newFakeImage(&p, "localhost:3030/tsuru/app1")
	c.Assert(err, check.IsNil)
	err = s.newFakeImage(&p, "localhost:3030/tsuru/app2")
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	app1 := &app.App{Name: "app1", Platform: "python", Deploys: 40}
	err = conn.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	ok := s.p.usePlatformImage(app1)
	c.Assert(ok, check.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app1"})
	app2 := &app.App{Name: "app2", Platform: "python", Deploys: 20}
	err = conn.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app2)
	c.Assert(ok, check.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app2"})
	app3 := &app.App{Name: "app3", Platform: "python", Deploys: 0}
	err = conn.Apps().Insert(app3)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app3)
	c.Assert(ok, check.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app3"})
	app4 := &app.App{Name: "app4", Platform: "python", Deploys: 19}
	err = conn.Apps().Insert(app4)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app4)
	c.Assert(ok, check.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app4"})
	app5 := &app.App{
		Name:           "app5",
		Platform:       "python",
		Deploys:        19,
		UpdatePlatform: true,
	}
	err = conn.Apps().Insert(app5)
	c.Assert(err, check.IsNil)
	ok = s.p.usePlatformImage(app5)
	c.Assert(ok, check.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app5"})
	app6 := &app.App{Name: "app6", Platform: "python", Deploys: 19}
	err = conn.Apps().Insert(app6)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": "app6"})
	coll := s.p.collection()
	defer coll.Close()
	err = coll.Insert(container{AppName: app6.Name, Image: "tsuru/app-app6"})
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
	c.Assert(platName, check.Equals, "tsuru/python")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	platName = platformImageName("ruby")
	c.Assert(platName, check.Equals, "localhost:3030/tsuru/ruby")
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
	yamlData, err := getImageTsuruYamlDataWithFallback(imgName, "")
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
	yamlData, err := getImageTsuruYamlDataWithFallback(imgName, "")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestGetImageTsuruYamlDataWithFallback(c *check.C) {
	data1 := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"after": []string{"cmd1", "cmd2"},
			},
		},
	}
	data2 := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"after": []string{"cmd3"},
			},
		},
	}
	a := &app.App{
		Name:       "mytestapp",
		CustomData: data1,
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	yamlData, err := getImageTsuruYamlDataWithFallback("tsuru/some-image", a.Name)
	c.Assert(err, check.IsNil)
	expected := provision.TsuruYamlData{
		Hooks: provision.TsuruYamlHooks{
			Restart: provision.TsuruYamlRestartHooks{
				After: []string{"cmd1", "cmd2"},
			},
		},
	}
	c.Assert(yamlData, check.DeepEquals, expected)
	// Overriden by image specific custom data
	err = saveImageCustomData("tsuru/some-image", data2)
	c.Assert(err, check.IsNil)
	yamlData, err = getImageTsuruYamlDataWithFallback("tsuru/some-image", a.Name)
	c.Assert(err, check.IsNil)
	expected.Hooks.Restart.After = []string{"cmd3"}
	c.Assert(yamlData, check.DeepEquals, expected)
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
	yamlData, err := getImageTsuruYamlDataWithFallback(img1Name, "")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}
