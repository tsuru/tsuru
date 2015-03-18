// Copyright 2015 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/cmd/cmdtest"
	tsuruIo "github.com/tsuru/tsuru/io"
	"gopkg.in/check.v1"
)

func (s *S) TestAddNodeToTheSchedulerCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:  "docker-node-add",
		Usage: "docker-node-add [param_name=param_value]... [--register]",
		Desc: `Creates or registers a new node in the cluster.
By default, this command will call the configured IaaS to create a new
machine. Every param will be sent to the IaaS implementation.

Parameters with special meaning:
  iaas=<iaas name>          Which iaas provider should be used, if not set
                            tsuru will use the default iaas specified in
                            tsuru.conf file.
  template=<template name>  A machine template with predefined parameters,
                            additional parameters will override template
                            ones. See 'machine-template-add' command.

--register: Registers an existing docker endpoint. The IaaS won't be called.
            Having a address=<docker_api_url> param is mandatory.
`,
		MinArgs: 0,
	}
	cmd := addNodeToSchedulerCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestAddNodeToTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool=poolTest", "address=http://localhost:8080"}, Stdout: &buf}
	expectedBody := `{"address":"http://localhost:8080","pool":"poolTest"}`
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			c.Assert(string(body), check.DeepEquals, expectedBody)
			return req.URL.Path == "/docker/node" && req.URL.RawQuery == "register=false"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addNodeToSchedulerCmd{register: false}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully registered.\n")
}

func (s *S) TestAddNodeWithErrorCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{
		Args:   []string{"pool=poolTest", "address=http://localhost:8080"},
		Stdout: &buf, Stderr: &buf,
	}
	expectedBody := `{"address":"http://localhost:8080","pool":"poolTest"}`
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: `{"error": "some err", "description": "my iaas desc"}`,
			Status:  http.StatusBadRequest,
		},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			c.Assert(string(body), check.DeepEquals, expectedBody)
			return req.URL.Path == "/docker/node" && req.URL.RawQuery == "register=false"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addNodeToSchedulerCmd{register: false}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Error: some err\n\nmy iaas desc\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdInfo(c *check.C) {
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
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
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
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully removed.\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerWithDestroyCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
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
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully removed.\n")
}

func (s *S) TestRemoveNodeFromTheSchedulerWithDestroyCmdRunConfirmation(c *check.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"http://localhost:8080"},
		Stdout: &stdout,
		Stdin:  strings.NewReader("n\n"),
	}
	command := removeNodeFromSchedulerCmd{}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Are you sure you sure you want to remove \"http://localhost:8080\" from cluster? (y/n) Abort.\n")
}

func (s *S) TestListNodesInTheSchedulerCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:  "docker-node-list",
		Usage: "docker-node-list [--filter/-f <metadata>=<value>]...",
		Desc:  "List available nodes in the cluster",
	}
	cmd := listNodesInTheSchedulerCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestListNodesInTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{
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
	err := (&listNodesInTheSchedulerCmd{}).Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `+------------------------+---------+----------+-----------+
| Address                | IaaS ID | Status   | Metadata  |
+------------------------+---------+----------+-----------+
| http://localhost1:8080 |         | disabled | meta1=foo |
|                        |         |          | meta2=bar |
+------------------------+---------+----------+-----------+
| http://localhost2:9090 | m-id-1  | ready    |           |
+------------------------+---------+----------+-----------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListNodesInTheSchedulerCmdRunWithFilters(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{
	"machines": [{"Id": "m-id-1", "Address": "localhost2"}],
	"nodes": [
		{"Address": "http://localhost1:8080", "Status": "disabled", "Metadata": {"meta1": "foo", "meta2": "bar"}},
		{"Address": "http://localhost2:9090", "Status": "disabled", "Metadata": {"meta1": "foo"}}
	]
}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := listNodesInTheSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"--filter", "meta1=foo", "-f", "meta2=bar"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `+------------------------+---------+----------+-----------+
| Address                | IaaS ID | Status   | Metadata  |
+------------------------+---------+----------+-----------+
| http://localhost1:8080 |         | disabled | meta1=foo |
|                        |         |          | meta2=bar |
+------------------------+---------+----------+-----------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListNodesInTheSchedulerCmdRunEmptyAll(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := (&listNodesInTheSchedulerCmd{}).Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `+---------+---------+--------+----------+
| Address | IaaS ID | Status | Metadata |
+---------+---------+--------+----------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
	cmd := listHealingHistoryCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
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

func (s *S) TestListHealingHistoryCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &listHealingHistoryCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
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
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr, endTStr, startTStr, endTStr, startTStr, endTStr)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunEmpty(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `[]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &listHealingHistoryCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `Node:
+-------+--------+---------+---------+---------+-------+
| Start | Finish | Success | Failing | Created | Error |
+-------+--------+---------+---------+---------+-------+
Container:
+-------+--------+---------+---------+---------+-------+
| Start | Finish | Success | Failing | Created | Error |
+-------+--------+---------+---------+---------+-------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunFilterNode(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing" && req.URL.RawQuery == "filter=node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &listHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--node"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
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
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunFilterContainer(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/healing" && req.URL.RawQuery == "filter=container"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &listHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--container"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	startT, _ := time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT, _ := time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startTStr := startT.Local().Format(time.Stamp)
	endTStr := endT.Local().Format(time.Stamp)
	expected := fmt.Sprintf(`Container:
+-----------------+-----------------+---------+------------+------------+-------+
| Start           | Finish          | Success | Failing    | Created    | Error |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr, endTStr, startTStr, endTStr)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListAutoScaleHistoryCmdRunEmpty(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `[]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/autoscale" && req.URL.Query().Get("skip") == "0" && req.URL.Query().Get("limit") == "20"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	autoscale := &listAutoScaleHistoryCmd{}
	err := autoscale.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `+-------+--------+---------+----------+--------+--------+-------+
| Start | Finish | Success | Metadata | Action | Reason | Error |
+-------+--------+---------+----------+--------+--------+-------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListAutoScaleHistoryCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	msg := `[{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Successful": true,
	"Action": "add",
	"Reason": "r1",
	"MetadataValue": "poolx",
	"Error": ""
},
{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Successful": false,
	"Action": "rebalance",
	"Reason": "r2",
	"MetadataValue": "poolx",
	"Error": "some error"
}]`
	startT, _ := time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT, _ := time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startTStr := startT.Local().Format(time.Stamp)
	endTStr := endT.Local().Format(time.Stamp)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: msg, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/autoscale"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	autoscale := &listAutoScaleHistoryCmd{}
	err := autoscale.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `+-----------------+-----------------+---------+----------+-----------+--------+------------+
| Start           | Finish          | Success | Metadata | Action    | Reason | Error      |
+-----------------+-----------------+---------+----------+-----------+--------+------------+
| %s | %s | true    | poolx    | add       | r1     |            |
+-----------------+-----------------+---------+----------+-----------+--------+------------+
| %s | %s | false   | poolx    | rebalance | r2     | some error |
+-----------------+-----------------+---------+----------+-----------+--------+------------+
`
	expected = fmt.Sprintf(expected, startTStr, endTStr, startTStr, endTStr)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestUpdateNodeToTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:1111", "x=y", "y=z"}, Stdout: &buf}
	expectedBody := map[string]string{
		"address": "http://localhost:1111",
		"x":       "y",
		"y":       "z",
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			var parsed map[string]string
			err := json.Unmarshal(body, &parsed)
			c.Assert(err, check.IsNil)
			c.Assert(parsed, check.DeepEquals, expectedBody)
			return req.URL.Path == "/docker/node" && req.Method == "PUT"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := updateNodeToSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully updated.\n")
}

func (s *S) TestListAutoScaleRunCmdRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	msg, _ := json.Marshal(tsuruIo.SimpleJsonMessage{Message: "progress msg"})
	result := string(msg)
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/autoscale/run" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cm := listAutoScaleRunCmd{}
	cm.Flags().Parse(true, []string{"-y"})
	err := cm.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "progress msg")
}
