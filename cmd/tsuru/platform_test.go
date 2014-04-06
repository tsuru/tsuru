// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestPlatformList(c *gocheck.C) {
	var buf bytes.Buffer
	var called bool
	transport := testing.ConditionalTransport{
		Transport: testing.Transport{
			Status:  http.StatusOK,
			Message: `[{"Name":"python"},{"Name":"ruby"}]`,
		},
		CondFunc: func(r *http.Request) bool {
			called = true
			return r.Method == "GET" && r.URL.Path == "/platforms"
		},
	}
	context := cmd.Context{Stdout: &buf}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := platformList{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := `- python
- ruby` + "\n"
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestPlatformListEmpty(c *gocheck.C) {
	var buf bytes.Buffer
	transport := testing.Transport{
		Status:  http.StatusOK,
		Message: `[]`,
	}
	context := cmd.Context{Stdout: &buf}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := platformList{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "No platforms available.\n")
}

func (s *S) TestPlatformListInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "platform-list",
		Usage:   "platform-list",
		Desc:    "Display the list of available platforms.",
		MinArgs: 0,
	}
	c.Assert(platformList{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPlatformListIsACommand(c *gocheck.C) {
	var _ cmd.Command = platformList{}
}
