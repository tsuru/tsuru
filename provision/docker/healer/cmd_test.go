// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"fmt"
	"net/http"
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
	"Successful": true,
	"Action": "node-healing",
	"FailingNode": {"Address": "addr1"},
	"CreatedNode": {"Address": "addr2"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T10:00:00.000Z",
	"EndTime": "2014-10-23T10:30:00.000Z",
	"Successful": false,
	"Action": "node-healing",
	"FailingNode": {"Address": "addr1"},
	"CreatedNode": {"Address": "addr2"},
	"Error": ""
},
{
	"StartTime": "2014-10-23T06:00:00.000Z",
	"EndTime": "2014-10-23T06:30:00.000Z",
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
},
{
	"StartTime": "2014-10-23T02:00:00.000Z",
	"EndTime": "2014-10-23T02:30:00.000Z",
	"Successful": false,
	"Action": "container-healing",
	"FailingContainer": {"ID": "123456789012"},
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
| %s | %s | false   | addr1   | addr2   |       |
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
| %s | %s | false   | addr1   | addr2   |       |
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

func (s *S) TestListHealingHistoryInProgressCmdRun(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf}
	msg := fmt.Sprintf(`[{
  	"StartTime": "2014-10-23T08:00:00.000Z",
  	"EndTime": "%s",
  	"Successful": true,
  	"Action": "container-healing",
    "FailingContainer": {"ID": "123456789012"},
    "CreatedContainer": {"ID": "923456789012"},
  	"Error": ""
  }]`, time.Time{}.Format(time.RFC3339))
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: msg, Status: http.StatusOK},
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
+-----------------+-------------+---------+------------+------------+-------+
| Start           | Finish      | Success | Failing    | Created    | Error |
+-----------------+-------------+---------+------------+------------+-------+
| %s | in progress | true    | 1234567890 | 9234567890 |       |
+-----------------+-------------+---------+------------+------------+-------+
`, startTStr08)
	c.Assert(buf.String(), check.Equals, expected)
}
