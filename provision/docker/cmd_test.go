// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"github.com/tsuru/tsuru/db"
	"launchpad.net/gocheck"
	"net/http"
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
	config.Set("docker:segregate", true)
	s.storage, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *CmdSuite) TearDownSuite(c *gocheck.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *CmdSuite) TestAddNodeToTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-node-add",
		Usage:   "docker-node-add <pool> <address>",
		Desc:    "Registers a new node in the cluster",
		MinArgs: 1,
	}
	cmd := addNodeToSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestAddNodeToTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"poolTest", "http://localhost:8080"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/node/add"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addNodeToSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully registered.\n")
}

func (s *CmdSuite) TestRemoveNodeFromTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-node-remove",
		Usage:   "docker-node-remove <pool> <address>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
	cmd := removeNodeFromSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestRemoveNodeFromTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool1", "http://localhost:8080"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/node/remove"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removeNodeFromSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *CmdSuite) TestListNodesInTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-node-list",
		Usage: "docker-node-list",
		Desc:  "List available nodes in the cluster",
	}
	cmd := listNodesInTheSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *CmdSuite) TestListNodesInTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: `[{"Address": "http://localhost:8080"}, {"Address": "http://localhost:9090"}]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listNodesInTheSchedulerCmd{}.Run(&context, client)
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
