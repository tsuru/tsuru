// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
)

type FlattenSuite struct {
	apps         []app.App
	conn         *db.Storage
	requests     []http.Request
	server       *dtesting.DockerServer
	clusterNodes map[string]string
}

var _ = gocheck.Suite(&FlattenSuite{})

func (s *FlattenSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_flatten")
	s.conn, err = db.Conn()
	units := []app.Unit{{Name: "4fa6e0f0c678"}, {Name: "e90e34656806"}}
	app1 := app.App{Name: "app1", Platform: "python", Deploys: 40, Units: units}
	err = s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	err = collection().Insert(&container{ID: "app1id", AppName: "app1", Image: "tsuru/app1"})
	c.Assert(err, gocheck.IsNil)
	app2 := app.App{Name: "app2", Platform: "python", Deploys: 20, Units: units}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	app3 := app.App{Name: "app3", Platform: "python", Deploys: 3, Units: units}
	err = s.conn.Apps().Insert(app3)
	c.Assert(err, gocheck.IsNil)
	app4 := app.App{Name: "app4", Platform: "python", Deploys: 19, Units: units}
	err = s.conn.Apps().Insert(app4)
	c.Assert(err, gocheck.IsNil)
	s.apps = append(s.apps, []app.App{app1, app2, app3, app4}...)
	var handler = func(r *http.Request) {
		s.requests = append(s.requests, *r)
	}
	s.server, err = dtesting.NewServer(handler)
	c.Assert(err, gocheck.IsNil)
	node := cluster.Node{ID: "server", Address: s.server.URL()}
	var scheduler segregatedScheduler
	dCluster, _ = cluster.New(&scheduler, node)
	dCluster.SetStorage(&mapStorage{})
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	err = newImage("tsuru/app1", s.server.URL())
	c.Assert(err, gocheck.IsNil)
}

func (s *FlattenSuite) TearDownSuite(c *gocheck.C) {
	s.server.Stop()
	collection().RemoveAll(nil)
	names := make([]string, len(s.apps))
	for i, a := range s.apps {
		names[i] = a.GetName()
	}
	_, err := s.conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": names}})
	c.Assert(err, gocheck.IsNil)
}

func (s *FlattenSuite) TearDownTest(c *gocheck.C) {
	s.requests = []http.Request{}
}

type mapStorage struct{}

func (m *mapStorage) StoreContainer(containerID, hostID string) error      { return nil }
func (m *mapStorage) RetrieveContainer(containerID string) (string, error) { return "", nil }
func (m *mapStorage) RemoveContainer(containerID string) error             { return nil }
func (m *mapStorage) StoreImage(imageID, hostID string) error              { return nil }
func (m *mapStorage) RetrieveImage(imageID string) (string, error)         { return "", nil }
func (m *mapStorage) RemoveImage(imageID string) error                     { return nil }

func (s *FlattenSuite) TestImagesToFlattenRetrievesOnlyUnitsWith20DeploysOrMore(c *gocheck.C) {
	images := imagesToFlatten()
	c.Assert(len(images), gocheck.Equals, 2)
	expected := []string{"tsuru/app1", "tsuru/python"}
	c.Assert(images, gocheck.DeepEquals, expected)
}

func (s *FlattenSuite) TestFlatten(c *gocheck.C) {
	Flatten()
	//c.Assert(len(s.requests), gocheck.Equals, 8) //create, export, import, remove old img, remove container TWICE
}
