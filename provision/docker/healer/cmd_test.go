// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"gopkg.in/check.v1"
)

func (s *S) TestListHealingHistoryCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
	cmd := ListHealingHistoryCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

var healingJsonData = `[{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Target": {"Name": "node"},
	"StartCustomData": {"node": {"Address": "addr1"} },
	"EndCustomData": {"Address": "addr2"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T10:00:00.000Z",
	"EndTime": "2014-10-23T10:30:00.000Z",
	"Target": {"Name": "node"},
	"StartCustomData": {"node": {"Address": "addr1"} },
	"EndCustomData": {"Address": "addr2"},
	"Error": "xx"
},
{
	"StartTime": "2014-10-23T06:00:00.000Z",
	"EndTime": "2014-10-23T06:30:00.000Z",
	"Target": {"Name": "container"},
	"StartCustomData": {"ID": "123456789012"},
	"EndCustomData": {"ID": "923456789012"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T08:00:00.000Z",
	"EndTime": "2014-10-23T08:30:00.000Z",
	"Target": {"Name": "container"},
	"StartCustomData": {"ID": "123456789012"},
	"Error": "err1"
},
{
	"StartTime": "2014-10-23T02:00:00.000Z",
	"EndTime": "2014-10-23T02:30:00.000Z",
	"Target": {"Name": "container"},
	"StartCustomData": {"ID": "123456789012"},
	"Error": "err1"
}]`

var (
	startT08, _ = time.Parse(time.RFC3339, "2014-10-23T08:00:00.000Z")
	endT08, _   = time.Parse(time.RFC3339, "2014-10-23T08:30:00.000Z")
	startT10, _ = time.Parse(time.RFC3339, "2014-10-23T10:00:00.000Z")
	endT10, _   = time.Parse(time.RFC3339, "2014-10-23T10:30:00.000Z")
	startT06, _ = time.Parse(time.RFC3339, "2014-10-23T06:00:00.000Z")
	endT06, _   = time.Parse(time.RFC3339, "2014-10-23T06:30:00.000Z")
	startT02, _ = time.Parse(time.RFC3339, "2014-10-23T02:00:00.000Z")
	endT02, _   = time.Parse(time.RFC3339, "2014-10-23T02:30:00.000Z")
	startTStr08 = startT08.Local().Format(time.Stamp)
	endTStr08   = endT08.Local().Format(time.Stamp)
	startTStr10 = startT10.Local().Format(time.Stamp)
	endTStr10   = endT10.Local().Format(time.Stamp)
	startTStr06 = startT06.Local().Format(time.Stamp)
	endTStr06   = endT06.Local().Format(time.Stamp)
	startTStr02 = startT02.Local().Format(time.Stamp)
	endTStr02   = endT02.Local().Format(time.Stamp)
)

func (s *S) TestListHealingHistoryCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &ListHealingHistoryCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := fmt.Sprintf(`Node:
+-----------------+-----------------+---------+---------+---------+-------+
| Start           | Finish          | Success | Failing | Created | Error |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | true    | addr1   | addr2   |       |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | false   | addr1   | addr2   | xx    |
+-----------------+-----------------+---------+---------+---------+-------+
Container:
+-----------------+-----------------+---------+------------+------------+-------+
| Start           | Finish          | Success | Failing    | Created    | Error |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr08, endTStr08, startTStr10, endTStr10, startTStr06, endTStr06, startTStr08, endTStr08, startTStr02, endTStr02)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunEmpty(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/healing"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &ListHealingHistoryCmd{}
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
			return req.URL.Path == "/1.0/docker/healing" && req.URL.RawQuery == "filter=node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &ListHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--node"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := fmt.Sprintf(`Node:
+-----------------+-----------------+---------+---------+---------+-------+
| Start           | Finish          | Success | Failing | Created | Error |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | true    | addr1   | addr2   |       |
+-----------------+-----------------+---------+---------+---------+-------+
| %s | %s | false   | addr1   | addr2   | xx    |
+-----------------+-----------------+---------+---------+---------+-------+
`, startTStr08, endTStr08, startTStr10, endTStr10)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestListHealingHistoryCmdRunFilterContainer(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: healingJsonData, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/healing" && req.URL.RawQuery == "filter=container"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &ListHealingHistoryCmd{}
	cmd.Flags().Parse(true, []string{"--container"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := fmt.Sprintf(`Container:
+-----------------+-----------------+---------+------------+------------+-------+
| Start           | Finish          | Success | Failing    | Created    | Error |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | true    | 1234567890 | 9234567890 |       |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
| %s | %s | false   | 1234567890 |            | err1  |
+-----------------+-----------------+---------+------------+------------+-------+
`, startTStr06, endTStr06, startTStr08, endTStr08, startTStr02, endTStr02)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestGetNodeHealingConfigCmd(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{
"": {"enabled": true, "maxunresponsivetime": 2},
"p1": {"enabled": false, "maxunresponsivetime": 2, "maxunresponsivetimeinherited": true},
"p2": {"enabled": true, "maxunresponsivetime": 3, "enabledinherited": true}
}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/healing/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &GetNodeHealingConfigCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `Default:
+------------------------+----------+
| Config                 | Value    |
+------------------------+----------+
| Enabled                | true     |
| Max unresponsive time  | 2s       |
| Max time since success | disabled |
+------------------------+----------+

Pool "p1":
+------------------------+----------+-----------+
| Config                 | Value    | Inherited |
+------------------------+----------+-----------+
| Enabled                | false    | false     |
| Max unresponsive time  | 2s       | true      |
| Max time since success | disabled | false     |
+------------------------+----------+-----------+

Pool "p2":
+------------------------+----------+-----------+
| Config                 | Value    | Inherited |
+------------------------+----------+-----------+
| Enabled                | true     | true      |
| Max unresponsive time  | 3s       | false     |
| Max time since success | disabled | false     |
+------------------------+----------+-----------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestGetNodeHealingConfigCmdEmpty(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.0/docker/healing/node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &GetNodeHealingConfigCmd{}
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	expected := `Default:
+------------------------+----------+
| Config                 | Value    |
+------------------------+----------+
| Enabled                | false    |
| Max unresponsive time  | disabled |
| Max time since success | disabled |
+------------------------+----------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestDeleteNodeHealingConfigCmd(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			req.ParseForm()
			return req.URL.Path == "/1.0/docker/healing/node" && req.Method == "DELETE" &&
				req.Form.Get("name") == "Enabled" && req.Form.Get("pool") == "p1"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &DeleteNodeHealingConfigCmd{}
	healing.Flags().Parse(true, []string{"--enabled", "--pool", "p1", "-y"})
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node healing configuration successfully removed.\n")
}

func (s *S) TestSetNodeHealingConfigCmd(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: `{}`, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			req.ParseForm()
			c.Assert(req.Form, check.DeepEquals, url.Values{
				"pool":                []string{"p1"},
				"MaxUnresponsiveTime": []string{"10"},
				"Enabled":             []string{"false"},
			})
			return req.URL.Path == "/1.0/docker/healing/node" && req.Method == "POST"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	healing := &SetNodeHealingConfigCmd{}
	healing.Flags().Parse(true, []string{"--pool", "p1", "--disable", "--max-unresponsive", "10"})
	err := healing.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Node healing configuration successfully updated.\n")
}
