// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestEnvGetInfo(c *C) {
	e := EnvGet{}
	i := e.Info()
	desc := `retrieve environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, Equals, "env-get")
	c.Assert(i.Usage, Equals, "env-get [--app appname] [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...")
	c.Assert(i.Desc, Equals, desc)
	c.Assert(i.MinArgs, Equals, 0)
}

func (s *S) TestEnvGetRun(c *C) {
	*appName = "someapp"
	var stdout, stderr bytes.Buffer
	result := "DATABASE_HOST=somehost\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvGetRunWithMultipleParams(c *C) {
	*appName = "someapp"
	var stdout, stderr bytes.Buffer
	result := "DATABASE_HOST=somehost\nDATABASE_USER=someuser"
	params := []string{"DATABASE_HOST", "DATABASE_USER"}
	context := cmd.Context{
		Args:   params,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvGetWithoutTheFlag(c *C) {
	var stdout, stderr bytes.Buffer
	result := "DATABASE_HOST=somehost\nDATABASE_USER=someuser"
	params := []string{"DATABASE_HOST", "DATABASE_USER"}
	context := cmd.Context{
		Args:   params,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/seek/env" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "seek"}
	err := (&EnvGet{GuessingCommand{g: fake}}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvSetInfo(c *C) {
	e := EnvSet{}
	i := e.Info()
	desc := `set environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, Equals, "env-set")
	c.Assert(i.Usage, Equals, "env-set <NAME=value> [NAME=value] ... [--app appname]")
	c.Assert(i.Desc, Equals, desc)
	c.Assert(i.MinArgs, Equals, 1)
}

func (s *S) TestEnvSetRun(c *C) {
	*appName = "someapp"
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST=somehost"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := (&EnvSet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvSetRunWithMultipleParams(c *C) {
	*appName = "someapp"
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST=somehost", "DATABASE_USER=user"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := (&EnvSet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvSetWithoutFlag(c *C) {
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST=somehost", "DATABASE_USER=user"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/otherapp/env" && req.Method == "POST"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "otherapp"}
	err := (&EnvSet{GuessingCommand{g: fake}}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestEnvUnsetInfo(c *C) {
	e := EnvUnset{}
	i := e.Info()
	desc := `unset environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, Equals, "env-unset")
	c.Assert(i.Usage, Equals, "env-unset <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2] ... [ENVIRONMENT_VARIABLEN] [--app appname]")
	c.Assert(i.Desc, Equals, desc)
	c.Assert(i.MinArgs, Equals, 1)
}

func (s *S) TestEnvUnsetRun(c *C) {
	*appName = "someapp"
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully unset\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := (&EnvUnset{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}
func (s *S) TestEnvUnsetWithoutFlag(c *C) {
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully unset\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/otherapp/env" && req.Method == "DELETE"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "otherapp"}
	err := (&EnvUnset{GuessingCommand{g: fake}}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, result)
}

func (s *S) TestRequestEnvUrl(c *C) {
	*appName = "someapp"
	result := "DATABASE_HOST=somehost"
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	args := []string{"DATABASE_HOST"}
	b, err := requestEnvUrl("GET", GuessingCommand{g: &FakeGuesser{name: "someapp"}}, args, client)
	c.Assert(err, IsNil)
	c.Assert(b, Equals, result)
}
