package main

import (
	"bytes"
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestEnvGetInfo(c *C) {
	e := EnvGet{}
	i := e.Info()
	c.Assert(i.Name, Equals, "env-get")
	c.Assert(i.Usage, Equals, "env-get <appname> [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...")
	c.Assert(i.Desc, Equals, "retrieve environment variables for an app.")
	c.Assert(i.MinArgs, Equals, 1)
}

func (s *S) TestEnvGetRun(c *C) {
	result := "DATABASE_HOST=somehost\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"someapp", "DATABASE_HOST"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestEnvGetRunWithMultipleParams(c *C) {
	result := "DATABASE_HOST=somehost\nDATABASE_USER=someuser"
	params := []string{"someapp", "DATABASE_HOST", "DATABASE_USER"}
	context := cmd.Context{
		Cmds:   []string{},
		Args:   params,
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestEnvSetInfo(c *C) {
	e := EnvSet{}
	i := e.Info()
	c.Assert(i.Name, Equals, "env-set")
	c.Assert(i.Usage, Equals, "env-set <appname> <NAME=value> [NAME=value] ...")
	c.Assert(i.Desc, Equals, "set environment variables for an app.")
	c.Assert(i.MinArgs, Equals, 2)
}

func (s *S) TestEnvSetRun(c *C) {
	result := "variable(s) successfully exported\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"someapp", "DATABASE_HOST=somehost"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvSet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestEnvSetRunWithMultipleParams(c *C) {
	result := "variable(s) successfully exported\n"
	params := []string{"someapp", "DATABASE_HOST=somehost", "DATABASE_USER=user"}
	context := cmd.Context{
		Cmds:   []string{},
		Args:   params,
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvSet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestEnvUnsetInfo(c *C) {
	e := EnvUnset{}
	i := e.Info()
	c.Assert(i.Name, Equals, "env-unset")
	c.Assert(i.Usage, Equals, "env-unset <appname> <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2]")
	c.Assert(i.Desc, Equals, "unset environment variables for an app.")
	c.Assert(i.MinArgs, Equals, 2)
}

func (s *S) TestEnvUnsetRun(c *C) {
	result := "variable(s) successfully unset\n"
	context := cmd.Context{
		Cmds:   []string{},
		Args:   []string{"someapp", "DATABASE_HOST"},
		Stdout: manager.Stdout,
		Stderr: manager.Stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvUnset{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestRequestEnvUrl(c *C) {
	result := "DATABASE_HOST=somehost"
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	args := []string{"someapp", "DATABASE_HOST"}
	b, err := requestEnvUrl("GET", args, client)
	c.Assert(err, IsNil)
	c.Assert(b, Equals, result)
}
