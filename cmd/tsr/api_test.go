// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestAPICmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "api",
		Usage:   "api",
		Desc:    "Starts the tsuru api webserver.",
		MinArgs: 0,
	}
	c.Assert(apiCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAPICmdIsACommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &apiCmd{}
}

func (s *S) TestAPICmdFlags(c *gocheck.C) {
	command := apiCmd{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--dry", "true"})
	flag := flagset.Lookup("dry")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "dry")
	c.Assert(flag.Usage, gocheck.Equals, "dry-run: does not start the server (for testing purpose)")
	c.Assert(flag.Value.String(), gocheck.Equals, "true")
	c.Assert(flag.DefValue, gocheck.Equals, "false")
	flagset.Parse(true, []string{"--config", "../etc/tsuru.conf"})
	flag = flagset.Lookup("config")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "config")
	c.Assert(flag.Usage, gocheck.Equals, "tsr api server config file.")
	c.Assert(flag.Value.String(), gocheck.Equals, "../etc/tsuru.conf")
	c.Assert(flag.DefValue, gocheck.Equals, "/etc/tsuru/tsuru.conf")
	flagset.Parse(true, []string{"-d", "true"})
	flag = flagset.Lookup("d")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "d")
	c.Assert(flag.Usage, gocheck.Equals, "dry-run: does not start the server (for testing purpose)")
	c.Assert(flag.Value.String(), gocheck.Equals, "true")
	c.Assert(flag.DefValue, gocheck.Equals, "false")
	flagset.Parse(true, []string{"-c", "../etc/tsuru.conf"})
	flag = flagset.Lookup("c")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "c")
	c.Assert(flag.Usage, gocheck.Equals, "tsr api server config file.")
	c.Assert(flag.Value.String(), gocheck.Equals, "../etc/tsuru.conf")
	c.Assert(flag.DefValue, gocheck.Equals, "/etc/tsuru/tsuru.conf")
}
