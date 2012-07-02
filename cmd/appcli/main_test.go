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

func (s *S) TestAppIsRegistered(c *C) {
	manager := buildManager("tsuru")
	app, ok := manager.Commands["app"]
	c.Assert(ok, Equals, true)
	c.Assert(app, FitsTypeOf, &App{})
}

func (s *S) TestAppRunIsRegistered(c *C) {
	manager := buildManager("tsuru")
	run, ok := manager.Commands["run"]
	c.Assert(ok, Equals, true)
	c.Assert(run, FitsTypeOf, &AppRun{})
}

func (s *S) TestEnvIsRegistered(c *C) {
	manager := buildManager("tsuru")
	env, ok := manager.Commands["env"]
	c.Assert(ok, Equals, true)
	c.Assert(env, FitsTypeOf, &Env{})
}

func (s *S) TestKeyIsRegistered(c *C) {
	manager := buildManager("tsuru")
	key, ok := manager.Commands["key"]
	c.Assert(ok, Equals, true)
	c.Assert(key, FitsTypeOf, &Key{})
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

func (s *S) TestServiceIsRegistered(c *C) {
	manager := buildManager("tsuru")
	service, ok := manager.Commands["service"]
	c.Assert(ok, Equals, true)
	c.Assert(service, FitsTypeOf, &Service{})
}
