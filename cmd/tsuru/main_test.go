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

func (s *S) TestEnvGetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	get, ok := manager.Commands["env-get"]
	c.Assert(ok, Equals, true)
	c.Assert(get, FitsTypeOf, &EnvGet{})
}

func (s *S) TestEnvSetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	set, ok := manager.Commands["env-set"]
	c.Assert(ok, Equals, true)
	c.Assert(set, FitsTypeOf, &EnvSet{})
}

func (s *S) TestEnvUnsetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	unset, ok := manager.Commands["env-unset"]
	c.Assert(ok, Equals, true)
	c.Assert(unset, FitsTypeOf, &EnvUnset{})
}

func (s *S) TestKeyAddIsRegistered(c *C) {
	manager := buildManager("tsuru")
	add, ok := manager.Commands["key-add"]
	c.Assert(ok, Equals, true)
	c.Assert(add, FitsTypeOf, &KeyAdd{})
}

func (s *S) TestKeyRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["key-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &KeyRemove{})
}

func (s *S) TestServiceIsRegistered(c *C) {
	manager := buildManager("tsuru")
	service, ok := manager.Commands["service"]
	c.Assert(ok, Equals, true)
	c.Assert(service, FitsTypeOf, &Service{})
}
