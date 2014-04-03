// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"launchpad.net/gocheck"
)

func (s *S) TestAppListIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru")
	list, ok := manager.Commands["app-list"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(list, gocheck.FitsTypeOf, tsuru.AppList{})
}

func (s *S) TestSetCNameIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru-admin")
	cname, ok := manager.Commands["set-cname"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(cname, gocheck.FitsTypeOf, &tsuru.SetCName{})
}

func (s *S) TestUnsetCNameIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru-admin")
	cname, ok := manager.Commands["unset-cname"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(cname, gocheck.FitsTypeOf, &tsuru.UnsetCName{})
}

func (s *S) TestTokenGenIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru-admin")
	token, ok := manager.Commands["token-gen"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(token, gocheck.FitsTypeOf, &tokenGen{})
}

func (s *S) TestLogRemoveIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru-admin")
	token, ok := manager.Commands["log-remove"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(token, gocheck.FitsTypeOf, &logRemove{})
}

func (s *S) TestChangeQuotaIsRegistered(c *gocheck.C) {
	manager := buildManager("tsuru-admin")
	token, ok := manager.Commands["quota-update"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(token, gocheck.FitsTypeOf, &changeQuota{})
}

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *gocheck.C) {
	baseManager := cmd.BuildBaseManager("tsuru", version, header, nil)
	manager := buildManager("tsuru")
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(command, gocheck.FitsTypeOf, instance)
	}
}
