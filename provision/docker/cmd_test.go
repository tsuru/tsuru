// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/check.v1"
)

func (s *S) TestAddNodeToTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"pool=poolTest", "address=http://localhost:8080"}, Stdout: &buf}
	expectedBody := `{"address":"http://localhost:8080","pool":"poolTest"}`
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			c.Assert(string(body), check.DeepEquals, expectedBody)
			return req.URL.Path == "/1.0/docker/node" && req.URL.RawQuery == "register=false"
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
		Transport: cmdtest.Transport{Message: `{"error": "some err"}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			body, _ := ioutil.ReadAll(req.Body)
			c.Assert(string(body), check.DeepEquals, expectedBody)
			return req.URL.Path == "/1.0/docker/node" && req.URL.RawQuery == "register=false"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addNodeToSchedulerCmd{register: false}
	err := cmd.Run(&context, client)
	c.Assert(err.Error(), check.Equals, "some err")
}

func (s *S) TestRemoveNodeFromTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			var result map[string]string
			json.NewDecoder(req.Body).Decode(&result)
			return req.URL.Path == "/1.0/docker/node" &&
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
			return req.URL.Path == "/1.0/docker/node" &&
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

func (s *S) TestRemoveNodeFromTheSchedulerWithNoRebalanceCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:8080"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			var result map[string]string
			json.NewDecoder(req.Body).Decode(&result)
			return req.URL.Path == "/1.0/docker/node" &&
				req.URL.Query().Get("no-rebalance") == "true" &&
				result["address"] == "http://localhost:8080"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removeNodeFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y", "--no-rebalance"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully removed.\n")
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
			return req.URL.Path == "/1.0/docker/node"
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
		{"Address": "http://localhost2:8089", "Status": "disabled"},
		{"Address": "http://localhost2:9090", "Status": "disabled", "Metadata": {"meta1": "foo"}}
	]
}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/node"
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
			return req.URL.Path == "/1.0/docker/node"
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
func (s *S) TestListNodesInTheSchedulerCmdRunWithFlagQ(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{
	"machines": [{"Id": "m-id-1", "Address": "localhost2"}],
	"nodes": [
		{"Address": "http://localhost1:8080", "Status": "disabled", "Metadata": {"meta1": "foo", "meta2": "bar"}},
		{"Address": "http://localhost1:8989", "Status": "disabled", "Metadata": {"meta2": "bar"}},
		{"Address": "http://localhost2:9090", "Status": "ready"}
	]
}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := listNodesInTheSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-q", "-f", "meta2=bar"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := "http://localhost1:8080\nhttp://localhost1:8989\n"
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListAutoScaleHistoryCmdRunEmpty(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `[]`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/autoscale" && req.URL.Query().Get("skip") == "0" && req.URL.Query().Get("limit") == "20"
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
			return req.URL.Path == "/1.0/docker/autoscale"
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
			return req.URL.Path == "/1.0/docker/node" && req.Method == "PUT" && req.URL.Query().Get("disabled") == "false"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := updateNodeToSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully updated.\n")
}

func (s *S) TestUpdateNodeToDisableCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:1111", "x=y", "y=z"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("disabled") == "true"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cm := updateNodeToSchedulerCmd{}
	cm.Flags().Parse(true, []string{"--disable"})
	err := cm.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully updated.\n")
}

func (s *S) TestUpdateNodeToEnabledCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:1111", "x=y", "y=z"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("enabled") == "true"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cm := updateNodeToSchedulerCmd{}
	cm.Flags().Parse(true, []string{"--enable"})
	err := cm.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node successfully updated.\n")
}

func (s *S) TestUpdateNodeToEnabledDisableCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"http://localhost:1111", "x=y", "y=z"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{
			Message: "You can't make a node enable and disable at the same time.",
			Status:  http.StatusBadRequest,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("enabled") == "true" && req.URL.Query().Get("disabled") == "true"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cm := updateNodeToSchedulerCmd{}
	cm.Flags().Parse(true, []string{"--enable", "--disable"})
	err := cm.Run(&context, client)
	c.Assert(err, check.NotNil)
}

func (s *S) TestAutoScaleRunCmdRun(c *check.C) {
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
			return req.URL.Path == "/1.0/docker/autoscale/run" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cm := autoScaleRunCmd{}
	cm.Flags().Parse(true, []string{"-y"})
	err := cm.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "progress msg")
}

func (s *S) TestAutoScaleInfoCmdRun(c *check.C) {
	var calls int
	config := `{"GroupByMetadata":"pool","Enabled":true}`
	configTransport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: config, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			calls++
			return req.URL.Path == "/1.0/docker/autoscale/config" && req.Method == "GET"
		},
	}
	rules := `[
	{
		"MetadataFilter":"pool1",
		"Enabled":true,
		"MaxContainerCount":6,
		"ScaleDownRatio":1.33,
		"PreventRebalance":false,
		"MaxMemoryRatio":1.20,
		"Error": ""
	},
	{
		"MetadataFilter":"pool2",
		"Enabled":true,
		"MaxContainerCount":13,
		"ScaleDownRatio":1.33,
		"PreventRebalance":true,
		"MaxMemoryRatio":0.9,
		"Error": ""
	},
	{
		"MetadataFilter":"pool3",
		"Enabled":false,
		"MaxContainerCount":50,
		"ScaleDownRatio":1.33,
		"PreventRebalance":false,
		"MaxMemoryRatio":1.20,
		"Error": "something went wrong"
	}
]`
	rulesTransport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: rules, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			calls++
			return req.URL.Path == "/1.0/docker/autoscale/rules" && req.Method == "GET"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	manager := cmd.Manager{}
	trans := cmdtest.MultiConditionalTransport{
		ConditionalTransports: []cmdtest.ConditionalTransport{configTransport, rulesTransport},
	}
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, &manager)
	var command autoScaleInfoCmd
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `Metadata filter: pool

Rules:
+--------------+---------------------+------------------+------------------+--------------------+---------+
| Filter value | Max container count | Max memory ratio | Scale down ratio | Rebalance on scale | Enabled |
+--------------+---------------------+------------------+------------------+--------------------+---------+
| pool1        | 6                   | 1.2000           | 1.3300           | true               | true    |
| pool2        | 13                  | 0.9000           | 1.3300           | false              | true    |
| pool3        | 50                  | 1.2000           | 1.3300           | true               | false   |
+--------------+---------------------+------------------+------------------+--------------------+---------+
`
	c.Assert(buf.String(), check.Equals, expected)
	c.Assert(calls, check.Equals, 2)
}

func (s *S) TestAutoScaleInfoCmdRunDisabled(c *check.C) {
	var calls int
	config := `{"GroupByMetadata":"pool","Enabled":false}`
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: config, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			calls++
			return req.URL.Path == "/1.0/docker/autoscale/config" && req.Method == "GET"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleInfoCmd
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "auto-scale is disabled\n")
	c.Assert(calls, check.Equals, 1)
}

func (s *S) TestAutoScaleSetRuleCmdRun(c *check.C) {
	var called bool
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			var rule autoScaleRule
			err := json.NewDecoder(req.Body).Decode(&rule)
			c.Assert(err, check.IsNil)
			c.Assert(rule, check.DeepEquals, autoScaleRule{
				MetadataFilter:    "pool1",
				Enabled:           true,
				MaxContainerCount: 10,
				MaxMemoryRatio:    1.2342,
				ScaleDownRatio:    1.33,
				PreventRebalance:  false,
			})
			return req.Method == "POST" && req.URL.Path == "/1.0/docker/autoscale/rules"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	var manager cmd.Manager
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleSetRuleCmd
	flags := []string{"-f", "pool1", "-c", "10", "-m", "1.2342"}
	err := command.Flags().Parse(true, flags)
	c.Assert(err, check.IsNil)
	err = command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	c.Assert(buf.String(), check.Equals, "Rule successfully defined.\n")
}

func (s *S) TestAutoScaleDeleteCmdRun(c *check.C) {
	var called bool
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/1.0/docker/autoscale/rules/myrule"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"myrule"}, Stdout: &buf}
	var manager cmd.Manager
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleDeleteRuleCmd
	err := command.Flags().Parse(true, []string{"-y"})
	c.Assert(err, check.IsNil)
	err = command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	c.Assert(buf.String(), check.Equals, "Rule successfully removed.\n")
}

func (s *S) TestAutoScaleDeleteCmdRunAskForConfirmation(c *check.C) {
	var called bool
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/1.0/docker/autoscale/rules/myrule"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"myrule"}, Stdout: &buf, Stdin: strings.NewReader("y\n")}
	var manager cmd.Manager
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleDeleteRuleCmd
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	c.Assert(buf.String(), check.Equals, "Are you sure you want to remove the rule \"myrule\"? (y/n) Rule successfully removed.\n")
}

func (s *S) TestAutoScaleDeleteCmdRunDefault(c *check.C) {
	var called bool
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/1.0/docker/autoscale/rules/"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	var manager cmd.Manager
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleDeleteRuleCmd
	err := command.Flags().Parse(true, []string{"-y"})
	c.Assert(err, check.IsNil)
	err = command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	c.Assert(buf.String(), check.Equals, "Rule successfully removed.\n")
}

func (s *S) TestAutoScaleDeleteCmdRunDefaultAskForConfirmation(c *check.C) {
	var called bool
	transport := cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			return req.Method == "DELETE" && req.URL.Path == "/1.0/docker/autoscale/rules/"
		},
	}
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf, Stdin: strings.NewReader("y\n")}
	var manager cmd.Manager
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, &manager)
	var command autoScaleDeleteRuleCmd
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	c.Assert(buf.String(), check.Equals, "Are you sure you want to remove the default rule? (y/n) Rule successfully removed.\n")
}

func (s *S) TestDockerLogUpdateRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := tsuruIo.SimpleJsonMessage{Message: "success!!!"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.Form, check.DeepEquals, url.Values{
				"restart":   []string{"false"},
				"pool":      []string{""},
				"Driver":    []string{"x"},
				"LogOpts.a": []string{"1"},
				"LogOpts.b": []string{"2"},
			})
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := dockerLogUpdate{}
	err := cmd.Flags().Parse(true, []string{"--log-driver", "x", "--log-opt", "a=1", "--log-opt", "b=2"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "success!!!")
}

func (s *S) TestDockerLogUpdateForPoolRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := tsuruIo.SimpleJsonMessage{Message: "success!!!"}
	result, _ := json.Marshal(msg)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			c.Assert(req.Form, check.DeepEquals, url.Values{
				"restart":   []string{"false"},
				"pool":      []string{"p1"},
				"Driver":    []string{"x"},
				"LogOpts.a": []string{"1"},
				"LogOpts.b": []string{"2"},
			})
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := dockerLogUpdate{}
	err := cmd.Flags().Parse(true, []string{"--pool", "p1", "--log-driver", "x", "--log-opt", "a=1", "--log-opt", "b=2"})
	c.Assert(err, check.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "success!!!")
}

func (s *S) TestDockerLogInfoRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	conf := map[string]container.DockerLogConfig{
		"":   {Driver: "x", LogOpts: map[string]string{"a": "1", "b": "2"}},
		"p1": {Driver: "x", LogOpts: map[string]string{"a": "9"}},
		"p2": {Driver: "bs"},
	}
	result, _ := json.Marshal(conf)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(result), Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/logs" && req.Method == "GET"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := dockerLogInfo{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, `Log driver [default]: x
+------+-------+
| Name | Value |
+------+-------+
| a    | 1     |
| b    | 2     |
+------+-------+

Log driver [pool p1]: x
+------+-------+
| Name | Value |
+------+-------+
| a    | 9     |
+------+-------+

Log driver [pool p2]: bs
`)
}
