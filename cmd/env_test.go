package cmd

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestEnvInfo(c *C) {
	e := Env{}
	i := e.Info()
	c.Assert(i.Name, Equals, "env")
	c.Assert(i.Usage, Equals, "env (get|set|unset)")
	c.Assert(i.Desc, Equals, "manage instance's environment variables.")
}

// func (s *S) TestEnvGetSubCommands(c *C) {
// 	e := Env{}
// 	sc := e.Subcommands()
// 	c.Assert(sc["get"], NotNil)
// }

func (s *S) TestEnvGetInfo(c *C) {
	e := EnvGet{}
	i := e.Info()
	c.Assert(i.Name, Equals, "get")
	c.Assert(i.Usage, Equals, "env get appname envname")
	c.Assert(i.Desc, Equals, "retrieve environment variables for an app.")
}
