// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"github.com/dotcloud/docker"
	dcli "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/cmd"
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
	config.Set("docker:repository-namespace", "tsuru")
	s.storage, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *SchedulerSuite) TearDownSuite(c *gocheck.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *SchedulerSuite) TestSchedulerSchedule(c *gocheck.C) {
	server0, err := testing.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer server0.Stop()
	server1, err := testing.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer server2.Stop()
	var buf bytes.Buffer
	client, _ := dcli.NewClient(server0.URL())
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/python"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/impius"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/mirror"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/dedication"}, &buf)
	client, _ = dcli.NewClient(server1.URL())
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/python"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/impius"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/mirror"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/dedication"}, &buf)
	client, _ = dcli.NewClient(server2.URL())
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/python"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/impius"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/mirror"}, &buf)
	client.PullImage(dcli.PullImageOptions{Repository: "tsuru/dedication"}, &buf)
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}}
	err = s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	err = coll.Insert(
		node{ID: "server0", Address: server0.URL(), Team: "tsuruteam"},
		node{ID: "server1", Address: server1.URL(), Team: "tsuruteam"},
		node{ID: "server2", Address: server2.URL()},
	)
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": bson.M{"$in": []string{"server0", "server1", "server2"}}})
	var scheduler segregatedScheduler
	config := docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/impius"}
	node, _, err := scheduler.Schedule(&config)
	c.Assert(err, gocheck.IsNil)
	c.Check(node, gocheck.Equals, "server2")
	config = docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/mirror"}
	node, _, err = scheduler.Schedule(&config)
	c.Assert(err, gocheck.IsNil)
	c.Check(node == "server0" || node == "server1", gocheck.Equals, true)
	config = docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/python"}
	node, _, err = scheduler.Schedule(&config)
	c.Assert(err, gocheck.IsNil)
	c.Check(node, gocheck.Equals, "server2")
	config = docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/dedication"}
	node, _, err = scheduler.Schedule(&config)
	c.Assert(err, gocheck.IsNil)
	c.Check(node, gocheck.Equals, "server2")
	config = docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}}
	node, _, _ = scheduler.Schedule(&config)
	c.Check(node, gocheck.Equals, "server2")
}

func (s *SchedulerSuite) TestSchedulerNoFallback(c *gocheck.C) {
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	config := docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/python"}
	var scheduler segregatedScheduler
	node, container, err := scheduler.Schedule(&config)
	c.Assert(node, gocheck.Equals, "")
	c.Assert(container, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, errNoFallback)
}

func (s *SchedulerSuite) TestSchedulerNoNamespace(c *gocheck.C) {
	old, _ := config.Get("docker:repository-namespace")
	defer config.Set("docker:repository-namespace", old)
	config.Unset("docker:repository-namespace")
	config := docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/python"}
	var scheduler segregatedScheduler
	node, container, err := scheduler.Schedule(&config)
	c.Assert(node, gocheck.Equals, "")
	c.Assert(container, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *SchedulerSuite) TestSchedulerInvalidEndpoint(c *gocheck.C) {
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	coll := s.storage.Collection(schedulerCollection)
	err = coll.Insert(node{ID: "server0", Address: "", Team: "jean"})
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "server0"})
	config := docker.Config{Cmd: []string{"/usr/sbin/sshd", "-D"}, Image: "tsuru/bill"}
	var scheduler segregatedScheduler
	node, container, err := scheduler.Schedule(&config)
	c.Assert(node, gocheck.Equals, "server0")
	c.Assert(container, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
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

func (s *SchedulerSuite) TestAddNodeToScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	err := addNodeToScheduler(nd, "team1")
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "server0"})
	var n node
	err = coll.Find(bson.M{"_id": "server0"}).One(&n)
	c.Assert(err, gocheck.IsNil)
	c.Check(n.ID, gocheck.Equals, "server0")
	c.Check(n.Team, gocheck.Equals, "team1")
	c.Check(n.Address, gocheck.Equals, "http://localhost:8080")
}

func (s *SchedulerSuite) TestAddNodeDuplicated(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	err := addNodeToScheduler(nd, "team1")
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "server0"})
	err = addNodeToScheduler(nd, "team2")
	c.Assert(err, gocheck.Equals, errNodeAlreadyRegister)
}

func (s *SchedulerSuite) TestRemoveNodeFromScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	err := addNodeToScheduler(nd, "team1")
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "server0"})
	err = removeNodeFromScheduler(nd)
	c.Assert(err, gocheck.IsNil)
	n, err := coll.Find(bson.M{"_id": "server0"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *SchedulerSuite) TesteRemoveUnknownNodeFromScheduler(c *gocheck.C) {
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	err := removeNodeFromScheduler(nd)
	c.Assert(err, gocheck.Equals, errNodeNotFound)
}

func (s *SchedulerSuite) TestListNodesInTheScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	nd1 := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	nd2 := cluster.Node{ID: "server1", Address: "http://localhost:9090"}
	nd3 := cluster.Node{ID: "server2", Address: "http://localhost:9090"}
	err := addNodeToScheduler(nd1, "team1")
	c.Assert(err, gocheck.IsNil)
	err = addNodeToScheduler(nd2, "team1")
	c.Assert(err, gocheck.IsNil)
	err = addNodeToScheduler(nd3, "team1")
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": bson.M{"$in": []string{"server0", "server1", "server2"}}})
	nodes, err := listNodesInTheScheduler()
	c.Assert(err, gocheck.IsNil)
	expected := []node{
		{ID: "server0", Address: "http://localhost:8080", Team: "team1"},
		{ID: "server1", Address: "http://localhost:9090", Team: "team1"},
		{ID: "server2", Address: "http://localhost:9090", Team: "team1"},
	}
	c.Assert(nodes, gocheck.DeepEquals, expected)
}

func (s *SchedulerSuite) TestAddNodeToTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-add-node",
		Usage:   "docker-add-node <id> <address> [team]",
		Desc:    "Registers a new node in the cluster, optionally assigning it to a team",
		MinArgs: 2,
	}
	cmd := addNodeToSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *SchedulerSuite) TestAddNodeToTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	context := cmd.Context{Args: []string{"server0", "http://localhost:8080"}, Stdout: &buf}
	cmd := addNodeToSchedulerCmd{}
	err := cmd.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "server0"})
	var n node
	err = coll.Find(bson.M{"_id": "server0"}).One(&n)
	c.Assert(err, gocheck.IsNil)
	c.Check(n.ID, gocheck.Equals, "server0")
	c.Check(n.Team, gocheck.Equals, "")
	c.Check(n.Address, gocheck.Equals, "http://localhost:8080")
	c.Assert(buf.String(), gocheck.Equals, "Node successfully registered.\n")
}

func (s *SchedulerSuite) TestAddNodeToTheSchedulerCmdRunWithTeam(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	context := cmd.Context{Args: []string{"server0", "http://localhost:8080", "team1"}, Stdout: &buf}
	cmd := addNodeToSchedulerCmd{}
	err := cmd.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "server0"})
	var n node
	err = coll.Find(bson.M{"_id": "server0"}).One(&n)
	c.Assert(err, gocheck.IsNil)
	c.Check(n.ID, gocheck.Equals, "server0")
	c.Check(n.Team, gocheck.Equals, "team1")
	c.Check(n.Address, gocheck.Equals, "http://localhost:8080")
	c.Assert(buf.String(), gocheck.Equals, "Node successfully registered.\n")
}

func (s *SchedulerSuite) TestAddNodeToTheSchedulerCmdFailure(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	err := addNodeToScheduler(cluster.Node{ID: "server0", Address: "http://localhost:4243"}, "")
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "server0"})
	context := cmd.Context{Args: []string{"server0", "http://localhost:8080", "team1"}, Stdout: &buf}
	cmd := addNodeToSchedulerCmd{}
	err = cmd.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
}

func (s *SchedulerSuite) TestRemoveNodeFromTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-rm-node",
		Usage:   "docker-rm-node <id>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
	cmd := removeNodeFromSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *SchedulerSuite) TestRemoveNodeFromTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	coll := s.storage.Collection(schedulerCollection)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	err := addNodeToScheduler(nd, "team1")
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(bson.M{"_id": "server0"})
	context := cmd.Context{Args: []string{"server0"}, Stdout: &buf}
	err = removeNodeFromSchedulerCmd{}.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	n, err := coll.Find(bson.M{"_id": "server0"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *SchedulerSuite) TestRemoveNodeFromTheSchedulerCmdRunFailure(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"server0"}, Stdout: &buf}
	err := removeNodeFromSchedulerCmd{}.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
}
