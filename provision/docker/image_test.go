// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"sort"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"gopkg.in/check.v1"
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "",
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
	err = s.conn.Apps().Insert(app1, app2, app3)
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL()})
	app1 := app.App{Name: "app1"}
	err = s.conn.Apps().Insert(app1)
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
	p.cluster, _ = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server.URL()})
	app1 := app.App{Name: "app1"}
	app2 := app.App{Name: "app2"}
	err = s.conn.Apps().Insert(app1, app2)
	c.Assert(err, check.IsNil)
	err = newFakeImage(&p, "localhost:3030/tsuru/app1", nil)
	c.Assert(err, check.IsNil)
	err = newFakeImage(&p, "localhost:3030/tsuru/app2", nil)
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = &p
	err = MigrateImages()
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	images, err := client.ListImages(docker.ListImagesOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(images, check.HasLen, 2)
	sort.Slice(images, func(i, j int) bool {
		return strings.Join(images[i].RepoTags, "") < strings.Join(images[j].RepoTags, "")
	})
	tags1 := images[0].RepoTags
	sort.Strings(tags1)
	tags2 := images[1].RepoTags
	sort.Strings(tags2)
	c.Assert(tags1, check.DeepEquals, []string{"localhost:3030/tsuru/app-app1", "localhost:3030/tsuru/app1"})
	c.Assert(tags2, check.DeepEquals, []string{"localhost:3030/tsuru/app-app2", "localhost:3030/tsuru/app2"})
}
