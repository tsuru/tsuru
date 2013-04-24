// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
	"net/http"
	"os"
)

func (s *S) TestTokenCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "token",
		Usage:   "token",
		Desc:    "Generates a tsuru token.",
		MinArgs: 0,
	}
	c.Assert(tokenCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTokenCmdIsACommand(c *gocheck.C) {
	var _ cmd.FlaggedCommand = &tokenCmd{}
}

func (s *S) TestTokenRun(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_tsr_test")
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	manager := cmd.NewManager("glb", "", "", &stdout, &stderr, os.Stdin)
	client := cmd.NewClient(&http.Client{}, nil, manager)
	command := tokenCmd{}
	command.Flags().Parse(true, []string{"--config", "../../etc/tsuru.conf"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Not(gocheck.Equals), "")
}

func (s *S) TestTokenCmdFlags(c *gocheck.C) {
	command := tokenCmd{}
	flagset := command.Flags()
	c.Assert(flagset, gocheck.NotNil)
	flagset.Parse(true, []string{"--config", "../etc/tsuru.conf"})
	flag := flagset.Lookup("config")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "config")
	c.Assert(flag.Usage, gocheck.Equals, "tsr collector config file.")
	c.Assert(flag.Value.String(), gocheck.Equals, "../etc/tsuru.conf")
	c.Assert(flag.DefValue, gocheck.Equals, "/etc/tsuru/tsuru.conf")
	flagset.Parse(true, []string{"-c", "../etc/tsuru.conf"})
	flag = flagset.Lookup("c")
	c.Assert(flag, gocheck.NotNil)
	c.Assert(flag.Name, gocheck.Equals, "c")
	c.Assert(flag.Usage, gocheck.Equals, "tsr collector config file.")
	c.Assert(flag.Value.String(), gocheck.Equals, "../etc/tsuru.conf")
	c.Assert(flag.DefValue, gocheck.Equals, "/etc/tsuru/tsuru.conf")
}
