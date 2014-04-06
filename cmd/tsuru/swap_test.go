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

func (s *S) TestSwapInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "swap",
		Usage:   "swap app1-name app2-name",
		Desc:    "Swap router between two apps.",
		MinArgs: 2,
	}
	c.Assert(swap{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestSwap(c *gocheck.C) {
	var buf bytes.Buffer
	var called bool
	transport := testing.ConditionalTransport{
		Transport: testing.Transport{Status: http.StatusOK, Message: ""},
		CondFunc: func(r *http.Request) bool {
			called = true
			return r.Method == "PUT" && r.URL.Path == "/swap"
		},
	}
	context := cmd.Context{
		Args:   []string{"app1", "app2"},
		Stdout: &buf,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	err := swap{}.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expected := "Apps successfully swapped!\n"
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestSwapIsACommand(c *gocheck.C) {
	var _ cmd.Command = swap{}
}
