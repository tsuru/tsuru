// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/tsuru/cmd"
	"gopkg.in/check.v1"
)

func (s *S) TestAPICmdInfo(c *check.C) {
	expected := &cmd.Info{
		Name:    "api",
		Usage:   "api",
		Desc:    "Starts the tsuru api webserver.",
		MinArgs: 0,
	}
	c.Assert(apiCmd{}.Info(), check.DeepEquals, expected)
}

func (s *S) TestAPICmdIsACommand(c *check.C) {
	var _ cmd.FlaggedCommand = &apiCmd{}
}

func (s *S) TestAPICmdFlags(c *check.C) {
	command := apiCmd{}
	flagset := command.Flags()
	c.Assert(flagset, check.NotNil)
	flagset.Parse(true, []string{"--dry", "true"})
	flag := flagset.Lookup("dry")
	c.Assert(flag, check.NotNil)
	c.Assert(flag.Name, check.Equals, "dry")
	c.Assert(flag.Usage, check.Equals, "dry-run: does not start the server (for testing purpose)")
	c.Assert(flag.Value.String(), check.Equals, "true")
	c.Assert(flag.DefValue, check.Equals, "false")
	flagset.Parse(true, []string{"-d", "true"})
	flag = flagset.Lookup("d")
	c.Assert(flag, check.NotNil)
	c.Assert(flag.Name, check.Equals, "d")
	c.Assert(flag.Usage, check.Equals, "dry-run: does not start the server (for testing purpose)")
	c.Assert(flag.Value.String(), check.Equals, "true")
	c.Assert(flag.DefValue, check.Equals, "false")
	flagset.Parse(true, []string{"-t", "true"})
	flag = flagset.Lookup("t")
	c.Assert(flag, check.NotNil)
	c.Assert(flag.Name, check.Equals, "t")
	c.Assert(flag.Usage, check.Equals, "check only config: test your tsuru.conf file before starts.")
}
