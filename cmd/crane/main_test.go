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

func (s *S) TestUpdateIsRegistered(c *C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["update"]
	c.Assert(ok, Equals, true)
	c.Assert(update, FitsTypeOf, &ServiceUpdate{})
}

func (s *S) TestDocGetIsRegistered(c *C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["doc-get"]
	c.Assert(ok, Equals, true)
	c.Assert(update, FitsTypeOf, &ServiceDocGet{})
}

func (s *S) TestDocAddIsRegistered(c *C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["doc-add"]
	c.Assert(ok, Equals, true)
	c.Assert(update, FitsTypeOf, &ServiceDocAdd{})
}

func (s *S) TestTemplateIsRegistered(c *C) {
	manager := buildManager("tsuru")
	update, ok := manager.Commands["template"]
	c.Assert(ok, Equals, true)
	c.Assert(update, FitsTypeOf, &ServiceTemplate{})
}
