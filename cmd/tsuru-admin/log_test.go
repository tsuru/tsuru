// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/testing"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestLogRemoveInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "log-remove",
		Usage:   "log-remove [--app appname]",
		Desc:    `remove all app logs.`,
		MinArgs: 0,
	}
	c.Assert((&logRemove{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestLogRemoveRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	expected := "Logs successfully removed!\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/logs" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := logRemove{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestLogRemoveByAppRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	expected := "Logs successfully removed!\n"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/logs" && req.Method == "DELETE" && req.URL.RawQuery == "app=app1"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := logRemove{}
	command.Flags().Parse(true, []string{"--app", "app1"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestLogRemoveFlagSet(c *gocheck.C) {
	command := logRemove{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--app", "ashamed"})
	app := flagset.Lookup("app")
	c.Check(app, gocheck.NotNil)
	c.Check(app.Name, gocheck.Equals, "app")
	c.Check(app.Usage, gocheck.Equals, "The name of the app.")
	c.Check(app.Value.String(), gocheck.Equals, "ashamed")
	c.Check(app.DefValue, gocheck.Equals, "")
	sapp := flagset.Lookup("a")
	c.Check(sapp, gocheck.NotNil)
	c.Check(sapp.Name, gocheck.Equals, "a")
	c.Check(sapp.Usage, gocheck.Equals, "The name of the app.")
	c.Check(sapp.Value.String(), gocheck.Equals, "ashamed")
	c.Check(sapp.DefValue, gocheck.Equals, "")
}