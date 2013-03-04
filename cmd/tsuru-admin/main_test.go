// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	. "launchpad.net/gocheck"
)

func (s *S) TestAppListIsRegistered(c *C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["app-list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, tsuru.AppList{})
}

func (s *S) TestSetCNameIsRegistered(c *C) {
	manager := buildManager("tsuru-admin")
	cname, ok := manager.Commands["set-cname"]
	c.Assert(ok, Equals, true)
	c.Assert(cname, FitsTypeOf, &tsuru.SetCName{})
}

func (s *S) TestUnsetCNameIsRegistered(c *C) {
	manager := buildManager("tsuru-admin")
	cname, ok := manager.Commands["unset-cname"]
	c.Assert(ok, Equals, true)
	c.Assert(cname, FitsTypeOf, &tsuru.UnsetCName{})
}

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *C) {
	baseManager := cmd.BuildBaseManager("tsuru", version, header)
	manager := buildManager("tsuru")
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, Equals, true)
		c.Assert(command, FitsTypeOf, instance)
	}
}
