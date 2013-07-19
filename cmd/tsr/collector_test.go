// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestCollectorCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "collector",
		Usage:   "collector",
		Desc:    "Starts the tsuru collector.",
		MinArgs: 0,
	}
	c.Assert(collectorCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestCollectorCmdIsACommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &collectorCmd{}
}

func (s *S) TestCollectorCmdFlags(c *gocheck.C) {
	command := collectorCmd{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--dry", "true"})
	flag := flagset.Lookup("dry")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "dry")
	c.Assert(flag.Usage, gocheck.Equals, "dry-run: does not run the collector (for testing purpose)")
	c.Assert(flag.Value.String(), gocheck.Equals, "true")
	c.Assert(flag.DefValue, gocheck.Equals, "false")
	flagset.Parse(true, []string{"-d", "true"})
	flag = flagset.Lookup("d")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "d")
	c.Assert(flag.Usage, gocheck.Equals, "dry-run: does not run the collector (for testing purpose)")
	c.Assert(flag.Value.String(), gocheck.Equals, "true")
	c.Assert(flag.DefValue, gocheck.Equals, "false")
}
