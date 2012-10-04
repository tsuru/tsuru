package main

import (
	"github.com/globocom/tsuru/cmd"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *C) {
	baseManager := cmd.BuildBaseManager("tsuru", version)
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

func (s *S) TestAppRestartIsRegistered(c *C) {
	manager := buildManager("tsuru")
	restart, ok := manager.Commands["restart"]
	c.Assert(ok, Equals, true)
	c.Assert(restart, FitsTypeOf, &AppRestart{})
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

func (s *S) TestServiceListIsRegistered(c *C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["service-list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &ServiceList{})
}

func (s *S) TestServiceAddIsRegistered(c *C) {
	manager := buildManager("tsuru")
	add, ok := manager.Commands["service-add"]
	c.Assert(ok, Equals, true)
	c.Assert(add, FitsTypeOf, &ServiceAdd{})
}

func (s *S) TestServiceRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["service-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &ServiceRemove{})
}

func (s *S) TestServiceBindIsRegistered(c *C) {
	manager := buildManager("tsuru")
	bind, ok := manager.Commands["bind"]
	c.Assert(ok, Equals, true)
	c.Assert(bind, FitsTypeOf, &ServiceBind{})
}

func (s *S) TestServiceUnbindIsRegistered(c *C) {
	manager := buildManager("tsuru")
	unbind, ok := manager.Commands["unbind"]
	c.Assert(ok, Equals, true)
	c.Assert(unbind, FitsTypeOf, &ServiceUnbind{})
}

func (s *S) TestServiceDocIsRegistered(c *C) {
	manager := buildManager("tsuru")
	doc, ok := manager.Commands["service-doc"]
	c.Assert(ok, Equals, true)
	c.Assert(doc, FitsTypeOf, &ServiceDoc{})
}

func (s *S) TestServiceInfoIsRegistered(c *C) {
	manager := buildManager("tsuru")
	info, ok := manager.Commands["service-info"]
	c.Assert(ok, Equals, true)
	c.Assert(info, FitsTypeOf, &ServiceInfo{})
}

func (s *S) TestServiceInstanceStatusIsRegistered(c *C) {
	manager := buildManager("tsuru")
	status, ok := manager.Commands["service-status"]
	c.Assert(ok, Equals, true)
	c.Assert(status, FitsTypeOf, &ServiceInstanceStatus{})
}

func (s *S) TestAppInfoIsRegistered(c *C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["app-info"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &AppInfo{})
}
