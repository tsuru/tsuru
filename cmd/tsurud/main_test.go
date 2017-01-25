// Copyright 2017 tsuru authors. All rights reserved.
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
	baseManager := cmd.NewManager("tsurud", "0.3.0", "", os.Stdout, os.Stderr, os.Stdin, nil)
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
	tsurudApi, ok := api.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsurudApi.Command, check.FitsTypeOf, &apiCmd{})
}

func (s *S) TestTokenCmdIsRegistered(c *check.C) {
	manager := buildManager()
	token, ok := manager.Commands["token"]
	c.Assert(ok, check.Equals, true)
	tsurudToken, ok := token.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsurudToken.Command, check.FitsTypeOf, tokenCmd{})
}

func (s *S) TestMigrateCmdIsRegistered(c *check.C) {
	manager := buildManager()
	cmd, ok := manager.Commands["migrate"]
	c.Assert(ok, check.Equals, true)
	migrate, ok := cmd.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(migrate.Command, check.FitsTypeOf, &migrateCmd{})
}

func (s *S) TestGandalfSyncCmdIsRegistered(c *check.C) {
	manager := buildManager()
	cmd, ok := manager.Commands["gandalf-sync"]
	c.Assert(ok, check.Equals, true)
	sync, ok := cmd.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(sync.Command, check.FitsTypeOf, gandalfSyncCmd{})
}

func (s *S) TestShouldRegisterAllCommandsFromProvisioners(c *check.C) {
	fp := provisiontest.NewFakeProvisioner()
	p := CommandableProvisioner{FakeProvisioner: fp}
	provision.Register("comm", func() (provision.Provisioner, error) { return &p, nil })
	manager := buildManager()
	fake, ok := manager.Commands["fake"]
	c.Assert(ok, check.Equals, true)
	tsurudFake, ok := fake.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(tsurudFake.Command, check.FitsTypeOf, &FakeCommand{})
}
