// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *check.C) {
	baseManager := cmd.NewManager("tsr", "0.3.0", "", os.Stdout, os.Stderr, os.Stdin, nil)
	manager := buildManager()
	for name, instance := range baseManager.Commands {
		command, ok := manager.Commands[name]
		c.Assert(ok, check.Equals, true)
		c.Assert(command, check.FitsTypeOf, instance)
	}
}

func (s *S) TestBuildManagerLoadsConfig(c *check.C) {
	buildManager()
	// As defined in testdata/tsuru.conf.
	listen, err := config.GetString("listen")
	c.Assert(err, check.IsNil)
	c.Assert(listen, check.Equals, "0.0.0.0:8080")
}

func (s *S) TestAPICmdIsRegistered(c *check.C) {
	manager := buildManager()
	api, ok := manager.Commands["api"]
	c.Assert(ok, check.Equals, true)
	tsrApi, ok := api.(*tsrCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsrApi.Command, check.FitsTypeOf, &apiCmd{})
}

func (s *S) TestTokenCmdIsRegistered(c *check.C) {
	manager := buildManager()
	token, ok := manager.Commands["token"]
	c.Assert(ok, check.Equals, true)
	tsrToken, ok := token.(*tsrCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsrToken.Command, check.FitsTypeOf, tokenCmd{})
}

func (s *S) TestShouldRegisterAllCommandsFromProvisioners(c *check.C) {
	fp := provisiontest.NewFakeProvisioner()
	p := CommandableProvisioner{FakeProvisioner: *fp}
	provision.Register("comm", &p)
	manager := buildManager()
	fake, ok := manager.Commands["fake"]
	c.Assert(ok, check.Equals, true)
	tsrFake, ok := fake.(*tsrCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsrFake.Command, check.FitsTypeOf, &FakeCommand{})
}
