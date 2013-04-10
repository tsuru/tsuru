// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/testing"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestTokenGen(c *gocheck.C) {
	var called bool
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"myapp"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	manager := cmd.NewManager("glb", "0.2", "ad-ver", &stdout, &stderr, nil)
	result := `{"token":"secret123"}`
	trans := testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			called = true
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			c.Assert(string(body), gocheck.Equals, `{"client":"myapp"}`)
			return req.Method == "POST" && req.URL.Path == "/tokens"
		},
	}
	expected := `Application token: "secret123".`
	client := cmd.NewClient(&http.Client{Transport: &trans}, nil, manager)
	command := tokenGen{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestTokenGenInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "token-gen",
		MinArgs: 1,
		Usage:   "token-gen <app-name>",
		Desc:    "Generates an authentication token for an app.",
	}
	c.Assert(tokenGen{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTokenGenIsACommand(c *gocheck.C) {
	var _ cmd.Command = tokenGen{}
}
