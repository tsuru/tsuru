// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *gocheck.C) {
	baseManager := cmd.NewManager("tsr", "0.3.0", "", os.Stdout, os.Stderr, os.Stdin, nil)
	manager := buildManager()
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(command, gocheck.FitsTypeOf, instance)
	}
}

func (s *S) TestBuildManagerLoadsConfig(c *gocheck.C) {
	buildManager()
	// As defined in testdata/tsuru.conf.
	listen, err := config.GetString("listen")
	c.Assert(err, gocheck.IsNil)
	c.Assert(listen, gocheck.Equals, "0.0.0.0:8080")
}

func (s *S) TestAPICmdIsRegistered(c *gocheck.C) {
	manager := buildManager()
	api, ok := manager.Commands["api"]
	c.Assert(ok, gocheck.Equals, true)
	tsrApi, ok := api.(*tsrCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(tsrApi.Command, gocheck.FitsTypeOf, &apiCmd{})
}

func (s *S) TestTokenCmdIsRegistered(c *gocheck.C) {
	manager := buildManager()
	token, ok := manager.Commands["token"]
	c.Assert(ok, gocheck.Equals, true)
	tsrToken, ok := token.(*tsrCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(tsrToken.Command, gocheck.FitsTypeOf, tokenCmd{})
}

func (s *S) TestShouldRegisterAllCommandsFromProvisioners(c *gocheck.C) {
	fp := testing.NewFakeProvisioner()
	p := CommandableProvisioner{FakeProvisioner: *fp}
	provision.Register("comm", &p)
	manager := buildManager()
	fake, ok := manager.Commands["fake"]
	c.Assert(ok, gocheck.Equals, true)
	tsrFake, ok := fake.(*tsrCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(tsrFake.Command, gocheck.FitsTypeOf, &FakeCommand{})
}

func (s *S) TestHealerCmdIsRegistered(c *gocheck.C) {
	manager := buildManager()
	healer, ok := manager.Commands["healer"]
	c.Assert(ok, gocheck.Equals, true)
	tsrHealer, ok := healer.(*tsrCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(tsrHealer.Command, gocheck.FitsTypeOf, &healerCmd{})
}
