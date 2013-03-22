// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestEnvGetInfo(c *gocheck.C) {
	e := EnvGet{}
	i := e.Info()
	desc := `retrieve environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, gocheck.Equals, "env-get")
	c.Assert(i.Usage, gocheck.Equals, "env-get [--app appname] [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...")
	c.Assert(i.Desc, gocheck.Equals, desc)
	c.Assert(i.MinArgs, gocheck.Equals, 0)
}

func (s *S) TestEnvGetRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := "DATABASE_HOST=somehost\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := EnvGet{}
	command.Flags().Parse(true, []string{"-a", "someapp"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvGetRunWithMultipleParams(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := "DATABASE_HOST=somehost\nDATABASE_USER=someuser"
	params := []string{"DATABASE_HOST", "DATABASE_USER"}
	context := cmd.Context{
		Args:   params,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := EnvGet{}
	command.Flags().Parse(true, []string{"-a", "someapp"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvGetWithoutTheFlag(c *gocheck.C) {
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
	err := (&EnvGet{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvSetInfo(c *gocheck.C) {
	e := EnvSet{}
	i := e.Info()
	desc := `set environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, gocheck.Equals, "env-set")
	c.Assert(i.Usage, gocheck.Equals, "env-set <NAME=value> [NAME=value] ... [--app appname]")
	c.Assert(i.Desc, gocheck.Equals, desc)
	c.Assert(i.MinArgs, gocheck.Equals, 1)
}

func (s *S) TestEnvSetRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST=somehost"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := EnvSet{}
	command.Flags().Parse(true, []string{"-a", "someapp"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvSetRunWithMultipleParams(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST=somehost", "DATABASE_USER=user"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := EnvSet{}
	command.Flags().Parse(true, []string{"-a", "someapp"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvSetWithoutFlag(c *gocheck.C) {
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
	err := (&EnvSet{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestEnvUnsetInfo(c *gocheck.C) {
	e := EnvUnset{}
	i := e.Info()
	desc := `unset environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`
	c.Assert(i.Name, gocheck.Equals, "env-unset")
	c.Assert(i.Usage, gocheck.Equals, "env-unset <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2] ... [ENVIRONMENT_VARIABLEN] [--app appname]")
	c.Assert(i.Desc, gocheck.Equals, desc)
	c.Assert(i.MinArgs, gocheck.Equals, 1)
}

func (s *S) TestEnvUnsetRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := "variable(s) successfully unset\n"
	context := cmd.Context{
		Args:   []string{"DATABASE_HOST"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	command := EnvUnset{}
	command.Flags().Parse(true, []string{"-a", "someapp"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}
func (s *S) TestEnvUnsetWithoutFlag(c *gocheck.C) {
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
	err := (&EnvUnset{GuessingCommand{G: fake}}).Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, result)
}

func (s *S) TestRequestEnvUrl(c *gocheck.C) {
	result := "DATABASE_HOST=somehost"
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	args := []string{"DATABASE_HOST"}
	g := GuessingCommand{G: &FakeGuesser{name: "someapp"}, appName: "something"}
	b, err := requestEnvUrl("GET", g, args, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, result)
}
