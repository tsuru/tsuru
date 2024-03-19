// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	check "gopkg.in/check.v1"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *check.C) {
	baseManager := cmd.NewManager("tsurud", os.Stdout, os.Stderr, os.Stdin, nil)
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

func (s *S) TestMigrateCmdIsRegistered(c *check.C) {
	manager := buildManager()
	cmd, ok := manager.Commands["migrate"]
	c.Assert(ok, check.Equals, true)
	migrate, ok := cmd.(*tsurudCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(migrate.Command, check.FitsTypeOf, &migrateCmd{})
}
