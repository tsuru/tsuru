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

func (s *S) TestAppCreateIsRegistered(c *C) {
	manager := buildManager("tsuru")
	create, ok := manager.Commands["app-create"]
	c.Assert(ok, Equals, true)
	c.Assert(create, FitsTypeOf, &AppCreate{})
}

func (s *S) TestAppRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["app-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &AppRemove{})
}

func (s *S) TestAppListIsRegistered(c *C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["app-list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &AppList{})
}

func (s *S) TestAppGrantIsRegistered(c *C) {
	manager := buildManager("tsuru")
	grant, ok := manager.Commands["app-grant"]
	c.Assert(ok, Equals, true)
	c.Assert(grant, FitsTypeOf, &AppGrant{})
}

func (s *S) TestAppRevokeIsRegistered(c *C) {
	manager := buildManager("tsuru")
	grant, ok := manager.Commands["app-revoke"]
	c.Assert(ok, Equals, true)
	c.Assert(grant, FitsTypeOf, &AppRevoke{})
}

func (s *S) TestAppLogIsRegistered(c *C) {
	manager := buildManager("tsuru")
	log, ok := manager.Commands["log"]
	c.Assert(ok, Equals, true)
	c.Assert(log, FitsTypeOf, &AppLog{})
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
