// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

type SchedulerSuite struct {
	storage *db.Storage
}

var _ = gocheck.Suite(&SchedulerSuite{})

func (s *SchedulerSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_scheduler_tests")
	s.storage, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *SchedulerSuite) TearDownSuite(c *gocheck.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *SchedulerSuite) TestSchedulerNodes(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(
		node{ID: "server0", Address: "http://localhost:8080", Team: "tsuru"},
		node{ID: "server1", Address: "http://localhost:8081", Team: "tsuru"},
		node{ID: "server2", Address: "http://localhost:8082", Team: "tsuru"},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": bson.M{"$in": []string{"server0", "server1", "server2"}}})
	expected := []cluster.Node{
		{ID: "server0", Address: "http://localhost:8080"},
		{ID: "server1", Address: "http://localhost:8081"},
		{ID: "server2", Address: "http://localhost:8082"},
	}
	var scheduler segregatedScheduler
	nodes, err := scheduler.Nodes()
	c.Assert(err, gocheck.IsNil)
	c.Assert(nodes, gocheck.DeepEquals, expected)
}
