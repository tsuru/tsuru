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
	healing := &ListHealingHistoryCmd{}
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
			return req.URL.Path == "/docker/healing" && req.URL.RawQuery == "filter=node"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := &ListHealingHistoryCmd{}
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
	cmd := &ListHealingHistoryCmd{}
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
