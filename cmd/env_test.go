package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestEnvInfo(c *C) {
	e := Env{}
	i := e.Info()
	c.Assert(i.Name, Equals, "env")
	c.Assert(i.Usage, Equals, "env (get|set|unset)")
	c.Assert(i.Desc, Equals, "manage instance's environment variables.")
}

func (s *S) TestEnvGetSubCommands(c *C) {
	e := Env{}
	sc := e.Subcommands()
	c.Assert(sc["get"], FitsTypeOf, &EnvGet{})
}

func (s *S) TestEnvGetInfo(c *C) {
	e := EnvGet{}
	i := e.Info()
	c.Assert(i.Name, Equals, "get")
	c.Assert(i.Usage, Equals, "env get appname envname")
	c.Assert(i.Desc, Equals, "retrieve environment variables for an app.")
}

func (s *S) TestEnvGetRun(c *C) {
	result := "DATABASE_HOST=somehost"
	context := Context{[]string{}, []string{"someapp", "DATABASE_HOST"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}

func (s *S) TestEnvGetRunWithMultipleParams(c *C) {
	result := "DATABASE_HOST=somehost\nDATABASE_USER=someuser"
	params := []string{"someapp", "DATABASE_HOST", "DATABASE_USER"}
	context := Context{[]string{}, params, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}})
	err := (&EnvGet{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, result)
}
