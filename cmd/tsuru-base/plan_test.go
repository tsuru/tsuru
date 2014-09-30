// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestPlanListInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plan-list",
		Usage:   "plan-list [--human]",
		Desc:    "List available plans that can be used when creating an app.",
		MinArgs: 0,
	}
	c.Assert((&PlanList{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPlanList(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[
    {"name": "test",  "memory": 536870912, "swap": 268435456, "cpushare": 100, "default": false},
    {"name": "test2", "memory": 536870912, "swap": 268435456, "cpushare": 200, "default": true}
]`
	expected := `+-------+-----------+-----------+-----------+---------+
| Name  | Memory    | Swap      | Cpu Share | Default |
+-------+-----------+-----------+-----------+---------+
| test  | 536870912 | 268435456 | 100       | false   |
| test2 | 536870912 | 268435456 | 200       | true    |
+-------+-----------+-----------+-----------+---------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/plans" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := PlanList{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestPlanListHuman(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[
    {"name": "test",  "memory": 536870912, "swap": 268435456, "cpushare": 100, "default": false},
    {"name": "test2", "memory": 536870912, "swap": 268435456, "cpushare": 200, "default": true}
]`
	expected := `+-------+--------+--------+-----------+---------+
| Name  | Memory | Swap   | Cpu Share | Default |
+-------+--------+--------+-----------+---------+
| test  | 512 MB | 256 MB | 100       | false   |
| test2 | 512 MB | 256 MB | 200       | true    |
+-------+--------+--------+-----------+---------+
`
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/plans" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := PlanList{}
	command.Flags().Parse(true, []string{"-h"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}
