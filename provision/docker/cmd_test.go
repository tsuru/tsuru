// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

type CmdSuite struct {
	storage *db.Storage
}

var _ = gocheck.Suite(&CmdSuite{})

func (s *CmdSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "docker_scheduler_tests")
	config.Set("docker:collection", "docker_unit_tests")
	s.storage, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *CmdSuite) TearDownSuite(c *gocheck.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *CmdSuite) TestAddNodeToTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-add-node",
		Usage:   "docker-add-node <id> <address> <pool>",
		Desc:    "Registers a new node in the cluster, optionally assigning it to a team",
		MinArgs: 3,
	}
	cmd := addNodeToSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestAddNodeToTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(Pool{Name: "poolTest"})
	c.Assert(err, gocheck.IsNil)
	context := cmd.Context{Args: []string{"server0", "http://localhost:8080", "poolTest"}, Stdout: &buf}
	cmd := addNodeToSchedulerCmd{}
	err = cmd.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "poolTest"})
	var n string
	var pool Pool
	err = coll.Find(bson.M{"_id": "poolTest"}).One(&pool)
	c.Assert(err, gocheck.IsNil)
	n = pool.Nodes[0]
	c.Check(pool.Name, gocheck.Equals, "poolTest")
	c.Check(pool.Nodes, gocheck.HasLen, 1)
	c.Check(len(pool.Teams), gocheck.Equals, 0)
	c.Check(n, gocheck.Equals, "http://localhost:8080")
	c.Assert(buf.String(), gocheck.Equals, "Node successfully registered.\n")
}

func (s *CmdSuite) TestAddNodeToTheSchedulerCmdFailure(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(Pool{Name: "poolTest"})
	c.Assert(err, gocheck.IsNil)
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"address": "http://localhost:4243", "pool": "poolTest"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "poolTest"})
	context := cmd.Context{Args: []string{"", "http://localhost:4243", "poolTest"}, Stdout: &buf}
	cmd := addNodeToSchedulerCmd{}
	err = cmd.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
}

func (s *CmdSuite) TestRemoveNodeFromTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-rm-node",
		Usage:   "docker-rm-node <id>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
	cmd := removeNodeFromSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestRemoveNodeFromTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(Pool{Name: "pool1"})
	c.Assert(err, gocheck.IsNil)
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"ID": "server0", "address": "http://localhost:8080", "pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "pool1"})
	context := cmd.Context{Args: []string{"pool1", "http://localhost:8080"}, Stdout: &buf}
	err = removeNodeFromSchedulerCmd{}.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": "pool1"}).One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(p.Nodes), gocheck.Equals, 0)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *CmdSuite) TestRemoveNodeFromTheSchedulerCmdRunFailure(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool1", "http://localhost:8080"}, Stdout: &buf}
	err := removeNodeFromSchedulerCmd{}.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
}

func (s *CmdSuite) TestListNodesInTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-list-nodes",
		Usage: "docker-list-nodes",
		Desc:  "List available nodes in the cluster",
	}
	cmd := listNodesInTheSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestListNodesInTheSchedulerCmdRun(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"ID": "server0", "address": "http://localhost:8080", "pool": pool.Name})
	c.Assert(err, gocheck.IsNil)
	err = scheduler.Register(map[string]string{"ID": "server1", "address": "http://localhost:9090", "pool": pool.Name})
	c.Assert(err, gocheck.IsNil)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": pool.Name})
	var buf bytes.Buffer
	ctx := cmd.Context{Stdout: &buf}
	err = listNodesInTheSchedulerCmd{}.Run(&ctx, nil)
	c.Assert(err, gocheck.IsNil)
	expected := `+-----------------------+
| Address               |
+-----------------------+
| http://localhost:8080 |
| http://localhost:9090 |
+-----------------------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}
