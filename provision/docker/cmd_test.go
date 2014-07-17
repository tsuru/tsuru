// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAddNodeToTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-node-add",
		Usage: "docker-node-add [param_name=param_value]... [--register]",
		Desc: `Creates or registers a new node in the cluster.
By default, this command will call the configured IaaS to create a new
machine. Every param will be sent to the IaaS implementation.

--register: Registers an existing docker endpoint. The IaaS won't be called.
            Having a address=<docker_api_url> param is mandatory.
`,
		MinArgs: 0,
	}
	cmd := addNodeToSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestAddNodeToTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool=poolTest", "address=http://localhost:8080"}, Stdout: &buf}
	expectedBody := `{"address":"http://localhost:8080","pool":"poolTest"}`
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			c.Assert(string(body), gocheck.DeepEquals, expectedBody)
			return req.URL.Path == "/docker/node" && req.URL.RawQuery == "register=false"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addNodeToSchedulerCmd{register: false}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully registered.\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-node-remove",
		Usage:   "docker-node-remove <address>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
	cmd := removeNodeFromSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool1", "http://localhost:8080"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removeNodeFromSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *S) TestListNodesInTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-node-list",
		Usage: "docker-node-list",
		Desc:  "List available nodes in the cluster",
	}
	cmd := listNodesInTheSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestListNodesInTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: `[
	{"Address": "http://localhost:8080", "Status": "disabled", "Metadata": {"meta1": "foo", "meta2": "bar"}},
	{"Address": "http://localhost:9090", "Status": "ready"}
]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listNodesInTheSchedulerCmd{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := `+-----------------------+----------+-----------+
| Address               | Status   | Metadata  |
+-----------------------+----------+-----------+
| http://localhost:8080 | disabled | meta1=foo |
|                       |          | meta2=bar |
+-----------------------+----------+-----------+
| http://localhost:9090 | ready    |           |
+-----------------------+----------+-----------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}
