// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
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

func (s *S) TestAddNodeWithErrorCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{
		Args:   []string{"pool=poolTest", "address=http://localhost:8080"},
		Stdout: &buf, Stderr: &buf,
	}
	expectedBody := `{"address":"http://localhost:8080","pool":"poolTest"}`
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: `{"error": "some err", "description": "my iaas desc"}`,
			Status:  http.StatusBadRequest,
		},
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
	c.Assert(buf.String(), gocheck.Equals, "Error: some err\n\nmy iaas desc\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-node-remove",
		Usage: "docker-node-remove <address> [--destroy] [-y]",
		Desc: `Removes a node from the cluster.

--destroy: Destroy the machine in the IaaS used to create it, if it exists.
`,
		MinArgs: 1,
	}
	cmd := removeNodeFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y"})
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			var result map[string]string
			json.NewDecoder(req.Body).Decode(&result)
			return req.URL.Path == "/docker/node" &&
				result["address"] == "http://localhost:8080"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removeNodeFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerWithDestroyCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			var result map[string]string
			json.NewDecoder(req.Body).Decode(&result)
			return req.URL.Path == "/docker/node" &&
				result["remove_iaas"] == "true" &&
				result["address"] == "http://localhost:8080"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removeNodeFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y", "--destroy"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Node successfully removed.\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerWithDestroyCmdRunConfirmation(c *gocheck.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"http://localhost:8080"},
		Stdout: &stdout,
		Stdin:  strings.NewReader("n\n"),
	}
	command := removeNodeFromSchedulerCmd{}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you sure you want to remove \"http://localhost:8080\" from cluster? (y/n) Abort.\n")
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
		Transport: testing.Transport{Message: `{
	"machines": [{"Id": "m-id-1", "Address": "localhost2"}],
	"nodes": [
		{"Address": "http://localhost1:8080", "Status": "disabled", "Metadata": {"meta1": "foo", "meta2": "bar"}},
		{"Address": "http://localhost2:9090", "Status": "ready"}
	]
}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listNodesInTheSchedulerCmd{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := `+------------------------+---------+----------+-----------+
| Address                | IaaS ID | Status   | Metadata  |
+------------------------+---------+----------+-----------+
| http://localhost1:8080 |         | disabled | meta1=foo |
|                        |         |          | meta2=bar |
+------------------------+---------+----------+-----------+
| http://localhost2:9090 | m-id-1  | ready    |           |
+------------------------+---------+----------+-----------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestListNodesInTheSchedulerCmdRunEmptyAll(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: `{}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listNodesInTheSchedulerCmd{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := `+---------+---------+--------+----------+
| Address | IaaS ID | Status | Metadata |
+---------+---------+--------+----------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
	cmd := listHealingHistoryCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

var healingJsonData = `[{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Successful": true,
	"Action": "node-healing",
	"FailingNode": {"Address": "addr1"},
	"CreatedNode": {"Address": "addr2"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Successful": true,
	"Action": "container-healing",
	"FailingContainer": {"ID": "123456789012"},
	"CreatedContainer": {"ID": "923456789012"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Successful": false,
	"Action": "container-healing",
	"FailingContainer": {"ID": "123456789012"},
	"Error": "err1"
}]`

func (s *S) TestListHealingHistoryCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &listHealingHistoryCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	startT, _ := time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT, _ := time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startTStr := startT.Local().Format(time.Stamp)
	endTStr := endT.Local().Format(time.Stamp)
	expected := fmt.Sprintf(`Node:
+-----------------+-----------------+---------+---------+---------+-------+
| Start           | Finish          | Success | Failing | Created | Error |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | true    | addr1   | addr2   |       |
+-----------------+-----------------+---------+---------+---------+-------+
Container:
+-----------------+-----------------+---------+------------+------------+-------+
| Start           | Finish          | Success | Failing    | Created    | Error |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr, endTStr, startTStr, endTStr, startTStr, endTStr)
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunEmpty(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: `[]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &listHealingHistoryCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := `Node:
+-------+--------+---------+---------+---------+-------+
| Start | Finish | Success | Failing | Created | Error |
+-------+--------+---------+---------+---------+-------+
Container:
+-------+--------+---------+---------+---------+-------+
| Start | Finish | Success | Failing | Created | Error |
+-------+--------+---------+---------+---------+-------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunFilterNode(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing" && req.URL.RawQuery == "filter=node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &listHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--node"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	startT, _ := time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT, _ := time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startTStr := startT.Local().Format(time.Stamp)
	endTStr := endT.Local().Format(time.Stamp)
	expected := fmt.Sprintf(`Node:
+-----------------+-----------------+---------+---------+---------+-------+
| Start           | Finish          | Success | Failing | Created | Error |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | true    | addr1   | addr2   |       |
+-----------------+-----------------+---------+---------+---------+-------+
`, startTStr, endTStr)
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunFilterContainer(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing" && req.URL.RawQuery == "filter=container"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &listHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--container"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	startT, _ := time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT, _ := time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startTStr := startT.Local().Format(time.Stamp)
	endTStr := endT.Local().Format(time.Stamp)
	expected := fmt.Sprintf(`Container:
+-----------------+-----------------+---------+------------+------------+-------+
| Start           | Finish          | Success | Failing    | Created    | Error |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr, endTStr, startTStr, endTStr)
	c.Assert(buf.String(), gocheck.Equals, expected)
}
