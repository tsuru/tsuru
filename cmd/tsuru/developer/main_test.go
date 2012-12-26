// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *C) {
	baseManager := cmd.BuildBaseManager("tsuru", version, header)
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
	c.Assert(list, FitsTypeOf, &tsuru.AppList{})
}

func (s *S) TestAppGrantIsRegistered(c *C) {
	manager := buildManager("tsuru")
	grant, ok := manager.Commands["app-grant"]
	c.Assert(ok, Equals, true)
	c.Assert(grant, FitsTypeOf, &tsuru.AppGrant{})
}

func (s *S) TestAppRevokeIsRegistered(c *C) {
	manager := buildManager("tsuru")
	grant, ok := manager.Commands["app-revoke"]
	c.Assert(ok, Equals, true)
	c.Assert(grant, FitsTypeOf, &tsuru.AppRevoke{})
}

func (s *S) TestAppLogIsRegistered(c *C) {
	manager := buildManager("tsuru")
	log, ok := manager.Commands["log"]
	c.Assert(ok, Equals, true)
	c.Assert(log, FitsTypeOf, &tsuru.AppLog{})
}

func (s *S) TestAppRunIsRegistered(c *C) {
	manager := buildManager("tsuru")
	run, ok := manager.Commands["run"]
	c.Assert(ok, Equals, true)
	c.Assert(run, FitsTypeOf, &tsuru.AppRun{})
}

func (s *S) TestAppRestartIsRegistered(c *C) {
	manager := buildManager("tsuru")
	restart, ok := manager.Commands["restart"]
	c.Assert(ok, Equals, true)
	c.Assert(restart, FitsTypeOf, &tsuru.AppRestart{})
}

func (s *S) TestEnvGetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	get, ok := manager.Commands["env-get"]
	c.Assert(ok, Equals, true)
	c.Assert(get, FitsTypeOf, &tsuru.EnvGet{})
}

func (s *S) TestEnvSetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	set, ok := manager.Commands["env-set"]
	c.Assert(ok, Equals, true)
	c.Assert(set, FitsTypeOf, &tsuru.EnvSet{})
}

func (s *S) TestEnvUnsetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	unset, ok := manager.Commands["env-unset"]
	c.Assert(ok, Equals, true)
	c.Assert(unset, FitsTypeOf, &tsuru.EnvUnset{})
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
	c.Assert(list, FitsTypeOf, &tsuru.ServiceList{})
}

func (s *S) TestServiceAddIsRegistered(c *C) {
	manager := buildManager("tsuru")
	add, ok := manager.Commands["service-add"]
	c.Assert(ok, Equals, true)
	c.Assert(add, FitsTypeOf, &tsuru.ServiceAdd{})
}

func (s *S) TestServiceRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["service-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(remove, FitsTypeOf, &tsuru.ServiceRemove{})
}

func (s *S) TestServiceBindIsRegistered(c *C) {
	manager := buildManager("tsuru")
	bind, ok := manager.Commands["bind"]
	c.Assert(ok, Equals, true)
	c.Assert(bind, FitsTypeOf, &tsuru.ServiceBind{})
}

func (s *S) TestServiceUnbindIsRegistered(c *C) {
	manager := buildManager("tsuru")
	unbind, ok := manager.Commands["unbind"]
	c.Assert(ok, Equals, true)
	c.Assert(unbind, FitsTypeOf, &tsuru.ServiceUnbind{})
}

func (s *S) TestServiceDocIsRegistered(c *C) {
	manager := buildManager("tsuru")
	doc, ok := manager.Commands["service-doc"]
	c.Assert(ok, Equals, true)
	c.Assert(doc, FitsTypeOf, &tsuru.ServiceDoc{})
}

func (s *S) TestServiceInfoIsRegistered(c *C) {
	manager := buildManager("tsuru")
	info, ok := manager.Commands["service-info"]
	c.Assert(ok, Equals, true)
	c.Assert(info, FitsTypeOf, &tsuru.ServiceInfo{})
}

func (s *S) TestServiceInstanceStatusIsRegistered(c *C) {
	manager := buildManager("tsuru")
	status, ok := manager.Commands["service-status"]
	c.Assert(ok, Equals, true)
	c.Assert(status, FitsTypeOf, &tsuru.ServiceInstanceStatus{})
}

func (s *S) TestAppInfoIsRegistered(c *C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["app-info"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &tsuru.AppInfo{})
}

func (s *S) TestUnitAddIsRegistered(c *C) {
	manager := buildManager("tsuru")
	addunit, ok := manager.Commands["unit-add"]
	c.Assert(ok, Equals, true)
	c.Assert(addunit, FitsTypeOf, &UnitAdd{})
}

func (s *S) TestUnitRemoveIsRegistered(c *C) {
	manager := buildManager("tsuru")
	rmunit, ok := manager.Commands["unit-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(rmunit, FitsTypeOf, &UnitRemove{})
}
