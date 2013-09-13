// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestHealerCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "healer",
		Usage:   "healer --host <tsuru-host:port>",
		Desc:    "Starts tsuru healer agent.",
		MinArgs: 1,
	}
	c.Assert((&healerCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestHealerCmdFlag(c *gocheck.C) {
	flagset := (&healerCmd{}).Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--host", "https://cloud.tsuru.io"})
	flag := flagset.Lookup("host")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "host")
	c.Assert(flag.Usage, gocheck.Equals, "host: tsuru host to discover and call healers on")
	c.Assert(flag.Value.String(), gocheck.Equals, "https://cloud.tsuru.io")
	c.Assert(flag.DefValue, gocheck.Equals, "")
	flag = flagset.Lookup("h")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "h")
	c.Assert(flag.Usage, gocheck.Equals, "host: tsuru host to discover and call healers on")
	c.Assert(flag.Value.String(), gocheck.Equals, "https://cloud.tsuru.io")
	c.Assert(flag.DefValue, gocheck.Equals, "")
}
