package main

import (
	"github.com/timeredbull/tsuru/cmd"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *C) {
	baseManager := cmd.BuildBaseManager("tsuru")
	manager := buildManager("tsuru")
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, Equals, true)
		c.Assert(command, FitsTypeOf, instance)
	}
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

func (s *S) TestServiceIsRegistered(c *C) {
	manager := buildManager("tsuru")
	service, ok := manager.Commands["service"]
	c.Assert(ok, Equals, true)
	c.Assert(service, FitsTypeOf, &Service{})
}

func (s *S) TestServiceInstanceStatusIsRegistered(c *C) {
	manager := buildManager("tsuru")
	service, ok := manager.Commands["status"]
	c.Assert(ok, Equals, true)
	c.Assert(service, FitsTypeOf, &ServiceInstanceStatus{})
}
