// Copyright 2014 tsuru authors. All rights reserved.
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
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestMigrateImages(c *gocheck.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldDockerCluster
	}()
	app1 := app.App{Name: "app1"}
	app2 := app.App{Name: "app2"}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1, app2)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name, app2.Name}}})
	err = newImage("tsuru/app1", "")
	c.Assert(err, gocheck.IsNil)
	err = newImage("tsuru/app2", "")
	c.Assert(err, gocheck.IsNil)
	err = migrateImages()
	c.Assert(err, gocheck.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, gocheck.IsNil)
	c.Assert(images, gocheck.HasLen, 2)
	tags1 := images[0].RepoTags
	sort.Strings(tags1)
	tags2 := images[1].RepoTags
	sort.Strings(tags2)
	c.Assert(tags1, gocheck.DeepEquals, []string{"tsuru/app-app1", "tsuru/app1"})
	c.Assert(tags2, gocheck.DeepEquals, []string{"tsuru/app-app2", "tsuru/app2"})
}

func (s *S) TestMigrateImagesWithoutImageInStorage(c *gocheck.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	cmutex.Lock()
	oldDockerCluster := dCluster
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server.URL()})
	cmutex.Unlock()
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldDockerCluster
	}()
	app1 := app.App{Name: "app1"}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name}}})
	err = migrateImages()
	c.Assert(err, gocheck.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, gocheck.IsNil)
	c.Assert(images, gocheck.HasLen, 0)
}

func (s *S) TestMigrateImagesWithRegistry(c *gocheck.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, gocheck.IsNil)
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
		defer cmutex.Unlock()
		dCluster = oldDockerCluster
	}()
	app1 := app.App{Name: "app1"}
	app2 := app.App{Name: "app2"}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Insert(app1, app2)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{app1.Name, app2.Name}}})
	err = newImage("localhost:3030/tsuru/app1", "")
	c.Assert(err, gocheck.IsNil)
	err = newImage("localhost:3030/tsuru/app2", "")
	c.Assert(err, gocheck.IsNil)
	err = migrateImages()
	c.Assert(err, gocheck.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, gocheck.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, gocheck.IsNil)
	c.Assert(images, gocheck.HasLen, 2)
	tags1 := images[0].RepoTags
	sort.Strings(tags1)
	tags2 := images[1].RepoTags
	sort.Strings(tags2)
	c.Assert(tags1, gocheck.DeepEquals, []string{"localhost:3030/tsuru/app-app1", "localhost:3030/tsuru/app1"})
	c.Assert(tags2, gocheck.DeepEquals, []string{"localhost:3030/tsuru/app-app2", "localhost:3030/tsuru/app2"})
}

func (s *S) TestUsePlatformImage(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	app1 := &app.App{Name: "app1", Platform: "python", Deploys: 40}
	err = conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	ok := usePlatformImage(app1)
	c.Assert(ok, gocheck.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app1"})
	app2 := &app.App{Name: "app2", Platform: "python", Deploys: 20}
	err = conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	ok = usePlatformImage(app2)
	c.Assert(ok, gocheck.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app2"})
	app3 := &app.App{Name: "app3", Platform: "python", Deploys: 0}
	err = conn.Apps().Insert(app3)
	c.Assert(err, gocheck.IsNil)
	ok = usePlatformImage(app3)
	c.Assert(ok, gocheck.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app3"})
	app4 := &app.App{Name: "app4", Platform: "python", Deploys: 19}
	err = conn.Apps().Insert(app4)
	c.Assert(err, gocheck.IsNil)
	ok = usePlatformImage(app4)
	c.Assert(ok, gocheck.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app4"})
	app5 := &app.App{
		Name:           "app5",
		Platform:       "python",
		Deploys:        19,
		UpdatePlatform: true,
	}
	err = conn.Apps().Insert(app5)
	c.Assert(err, gocheck.IsNil)
	ok = usePlatformImage(app5)
	c.Assert(ok, gocheck.Equals, true)
	defer conn.Apps().Remove(bson.M{"name": "app5"})
	app6 := &app.App{Name: "app6", Platform: "python", Deploys: 19}
	err = conn.Apps().Insert(app6)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": "app6"})
	coll := collection()
	defer coll.Close()
	err = coll.Insert(container{AppName: app6.Name, Image: "tsuru/app-app6"})
	c.Assert(err, gocheck.IsNil)
	ok = usePlatformImage(app6)
	c.Assert(ok, gocheck.Equals, false)
}

func (s *S) TestAppNewImageName(c *gocheck.C) {
	img1, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img1, gocheck.Equals, "tsuru/app-myapp:v1")
	img2, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img2, gocheck.Equals, "tsuru/app-myapp:v2")
	img3, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img3, gocheck.Equals, "tsuru/app-myapp:v3")
}

func (s *S) TestAppNewImageNameWithRegistry(c *gocheck.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	img1, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img1, gocheck.Equals, "localhost:3030/tsuru/app-myapp:v1")
	img2, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img2, gocheck.Equals, "localhost:3030/tsuru/app-myapp:v2")
	img3, err := appNewImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img3, gocheck.Equals, "localhost:3030/tsuru/app-myapp:v3")
}

func (s *S) TestAppCurrentImageNameWithoutImage(c *gocheck.C) {
	img1, err := appCurrentImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img1, gocheck.Equals, "tsuru/app-myapp")
}

func (s *S) TestAppCurrentImageName(c *gocheck.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, gocheck.IsNil)
	img1, err := appCurrentImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img1, gocheck.Equals, "tsuru/app-myapp:v1")
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, gocheck.IsNil)
	img2, err := appCurrentImageName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(img2, gocheck.Equals, "tsuru/app-myapp:v2")
}

func (s *S) TestListAppImages(c *gocheck.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, gocheck.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, gocheck.IsNil)
	images, err := listAppImages("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(images, gocheck.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:v2"})
}

func (s *S) TestValidListAppImages(c *gocheck.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, gocheck.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, gocheck.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, gocheck.IsNil)
	images, err := listValidAppImages("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(images, gocheck.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v3"})
}

func (s *S) TestPlatformImageName(c *gocheck.C) {
	platName := platformImageName("python")
	c.Assert(platName, gocheck.Equals, "tsuru/python")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	platName = platformImageName("ruby")
	c.Assert(platName, gocheck.Equals, "localhost:3030/tsuru/ruby")
}

func (s *S) TestDeleteAllAppImageNames(c *gocheck.C) {
	err := appendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, gocheck.IsNil)
	err = appendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, gocheck.IsNil)
	err = deleteAllAppImageNames("myapp")
	c.Assert(err, gocheck.IsNil)
	_, err = listAppImages("myapp")
	c.Assert(err, gocheck.ErrorMatches, "not found")
}
