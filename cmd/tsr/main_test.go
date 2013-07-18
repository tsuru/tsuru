// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
	"os"
)

func (s *S) TestCommandsFromBaseManagerAreRegistered(c *gocheck.C) {
	baseManager := cmd.NewManager("tsr", "0.1.0", "", os.Stdout, os.Stderr, os.Stdin)
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
	create, ok := manager.Commands["api"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(create, gocheck.FitsTypeOf, &apiCmd{})
}

func (s *S) TestCollectorCmdIsRegistered(c *gocheck.C) {
	manager := buildManager()
	create, ok := manager.Commands["collector"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(create, gocheck.FitsTypeOf, &collectorCmd{})
}

func (s *S) TestTokenCmdIsRegistered(c *gocheck.C) {
	manager := buildManager()
	create, ok := manager.Commands["token"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(create, gocheck.FitsTypeOf, &tokenCmd{})
}
