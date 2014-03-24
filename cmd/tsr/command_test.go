// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/config"
	"launchpad.net/gnuflag"
	"launchpad.net/gocheck"
)

func (s *S) TestConfigFileValueString(c *gocheck.C) {
	value := "/etc/tsuru/tsuru.conf"
	file := configFile{value: value}
	c.Assert(file.String(), gocheck.Equals, value)
}

func (s *S) TestConfigFileValueSet(c *gocheck.C) {
	var file configFile
	err := file.Set("testdata/tsuru2.conf")
	c.Assert(err, gocheck.IsNil)
	listen, err := config.GetString("listen")
	c.Assert(err, gocheck.IsNil)
	c.Assert(listen, gocheck.Equals, "localhost:8080")
	c.Assert(file.String(), gocheck.Equals, "testdata/tsuru2.conf")
}

func (s *S) TestConfigFileValueSetError(c *gocheck.C) {
	var file configFile
	err := file.Set("testdata/tsuru3.conf")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestTsrCommandFlagged(c *gocheck.C) {
	var originalCmd fakeCommand
	cmd := tsrCommand{Command: &originalCmd}
	flags := cmd.Flags()
	err := flags.Parse(true, []string{"--name", "Chico", "--config", "testdata/tsuru.conf"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(originalCmd.name, gocheck.Equals, "Chico")
	c.Assert(cmd.file.String(), gocheck.Equals, "testdata/tsuru.conf")
	cmd = tsrCommand{Command: &originalCmd}
	flags.Parse(true, []string{"--name", "Chico", "-c", "testdata/tsuru.conf"})
	c.Assert(cmd.file.String(), gocheck.Equals, "testdata/tsuru.conf")
}

func (s *S) TestTsrCommandNotFlagged(c *gocheck.C) {
	var originalCmd tokenCmd
	cmd := tsrCommand{Command: originalCmd}
	flags := cmd.Flags()
	err := flags.Parse(true, []string{"--config", "testdata/tsuru.conf"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmd.file.String(), gocheck.Equals, "testdata/tsuru.conf")
	cmd = tsrCommand{Command: originalCmd}
	flags.Parse(true, []string{"-c", "testdata/tsuru.conf"})
	c.Assert(cmd.file.String(), gocheck.Equals, "testdata/tsuru.conf")
}

type fakeCommand struct {
	tokenCmd
	name string
	fs   *gnuflag.FlagSet
}

func (f *fakeCommand) Flags() *gnuflag.FlagSet {
	if f.fs == nil {
		f.fs = gnuflag.NewFlagSet("fakeCommand", gnuflag.ExitOnError)
		f.fs.StringVar(&f.name, "name", "", "your name")
	}
	return f.fs
}
