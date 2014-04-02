// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *gocheck.C) {
	baseManager := cmd.BuildBaseManager("tsuru", version, header, nil)
	manager := buildManager("tsuru")
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(command, gocheck.FitsTypeOf, instance)
	}
}

func (s *S) TestCreateIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	target, ok := manager.Commands["create"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(target, gocheck.FitsTypeOf, &ServiceCreate{})
}

func (s *S) TestRemoveIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["remove"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(remove, gocheck.FitsTypeOf, &ServiceRemove{})
}

func (s *S) TestListIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	remove, ok := manager.Commands["list"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(remove, gocheck.FitsTypeOf, &ServiceList{})
}

func (s *S) TestUpdateIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["update"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(update, gocheck.FitsTypeOf, &ServiceUpdate{})
}

func (s *S) TestDocGetIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["doc-get"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(update, gocheck.FitsTypeOf, &ServiceDocGet{})
}

func (s *S) TestDocAddIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["doc-add"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(update, gocheck.FitsTypeOf, &ServiceDocAdd{})
}

func (s *S) TestTemplateIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["template"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(update, gocheck.FitsTypeOf, &ServiceTemplate{})
}
