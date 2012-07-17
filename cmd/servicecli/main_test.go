package main

import (
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
)

func (s *S) TestLoginIsRegistered(c *C) {
	manager := buildManager("tsuru")
	login, ok := manager.Commands["login"]
	c.Assert(ok, Equals, true)
	c.Assert(login, FitsTypeOf, &cmd.Login{})
}

func (s *S) TestLogoutIsRegistered(c *C) {
	manager := buildManager("tsuru")
	logout, ok := manager.Commands["logout"]
	c.Assert(ok, Equals, true)
	c.Assert(logout, FitsTypeOf, &cmd.Logout{})
}

func (s *S) TestUserIsRegistered(c *C) {
	manager := buildManager("tsuru")
	user, ok := manager.Commands["user"]
	c.Assert(ok, Equals, true)
	c.Assert(user, FitsTypeOf, &cmd.User{})
}

func (s *S) TestTeamIsRegistered(c *C) {
	manager := buildManager("tsuru")
	team, ok := manager.Commands["team"]
	c.Assert(ok, Equals, true)
	c.Assert(team, FitsTypeOf, &cmd.Team{})
}

func (s *S) TestTargetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	target, ok := manager.Commands["target"]
	c.Assert(ok, Equals, true)
	c.Assert(target, FitsTypeOf, &cmd.Target{})
}

func (s *S) TestCreateIsRegistered(c *C) {
	manager := buildManager("tsuru")
	target, ok := manager.Commands["create"]
	c.Assert(ok, Equals, true)
	c.Assert(target, FitsTypeOf, &ServiceCreate{})
}

func (s *S) TestRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["remove"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &ServiceRemove{})
}

func (s *S) TestListIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["list"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &ServiceList{})
}
