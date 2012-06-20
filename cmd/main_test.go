package main

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestLoginIsRegistered(c *C) {
	manager := buildManager("tsuru")
	login, ok := manager.commands["login"]
	c.Assert(ok, Equals, true)
	c.Assert(login, FitsTypeOf, &Login{})
}

func (s *S) TestLogoutIsRegistered(c *C) {
	manager := buildManager("tsuru")
	logout, ok := manager.commands["logout"]
	c.Assert(ok, Equals, true)
	c.Assert(logout, FitsTypeOf, &Logout{})
}

func (s *S) TestUserIsRegistered(c *C) {
	manager := buildManager("tsuru")
	user, ok := manager.commands["user"]
	c.Assert(ok, Equals, true)
	c.Assert(user, FitsTypeOf, &User{})
}

func (s *S) TestAppIsRegistered(c *C) {
	manager := buildManager("tsuru")
	app, ok := manager.commands["app"]
	c.Assert(ok, Equals, true)
	c.Assert(app, FitsTypeOf, &App{})
}

func (s *S) TestAppRunIsRegistered(c *C) {
	manager := buildManager("tsuru")
	run, ok := manager.commands["run"]
	c.Assert(ok, Equals, true)
	c.Assert(run, FitsTypeOf, &AppRun{})
}

func (s *S) TestEnvIsRegistered(c *C) {
	manager := buildManager("tsuru")
	env, ok := manager.commands["env"]
	c.Assert(ok, Equals, true)
	c.Assert(env, FitsTypeOf, &Env{})
}

func (s *S) TestKeyIsRegistered(c *C) {
	manager := buildManager("tsuru")
	key, ok := manager.commands["key"]
	c.Assert(ok, Equals, true)
	c.Assert(key, FitsTypeOf, &Key{})
}

func (s *S) TestTeamIsRegistered(c *C) {
	manager := buildManager("tsuru")
	team, ok := manager.commands["team"]
	c.Assert(ok, Equals, true)
	c.Assert(team, FitsTypeOf, &Team{})
}

func (s *S) TestTargetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	target, ok := manager.commands["target"]
	c.Assert(ok, Equals, true)
	c.Assert(target, FitsTypeOf, &Target{})
}

func (s *S) TestExtractProgramNameWithAbsolutePath(c *C) {
	got := extractProgramName("/usr/bin/tsuru")
	c.Assert(got, Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithRelativePath(c *C) {
	got := extractProgramName("./tsuru")
	c.Assert(got, Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithinThePATH(c *C) {
	got := extractProgramName("tsuru")
	c.Assert(got, Equals, "tsuru")
}
