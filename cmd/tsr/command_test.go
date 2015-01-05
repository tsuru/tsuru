// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"

	"github.com/tsuru/tsuru/cmd"
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
	c.Assert(file.String(), gocheck.Equals, "testdata/tsuru2.conf")
	c.Assert(configPath, gocheck.Equals, "testdata/tsuru2.conf")
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

func (s *S) TestTsrCommandRunInvalidConfig(c *gocheck.C) {
	fakeCmd := FakeCommand{}
	command := tsrCommand{Command: &fakeCmd}
	command.Flags().Parse(true, []string{"-c", "/invalid/file"})
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stderr.String(), gocheck.Equals, `Opening config file: /invalid/file
Could not open tsuru config file at /invalid/file (open /invalid/file: no such file or directory).
  For an example, see: tsuru/etc/tsuru.conf
  Note that you can specify a different config file with the --config option -- e.g.: --config=./etc/tsuru.conf
`)
	c.Assert(stdout.String(), gocheck.Equals, "")
	c.Assert(fakeCmd.Calls(), gocheck.Equals, int32(0))
}

func (s *S) TestTsrCommandRun(c *gocheck.C) {
	fakeCmd := FakeCommand{}
	command := tsrCommand{Command: &fakeCmd}
	command.Flags().Parse(true, []string{"-c", "testdata/tsuru.conf"})
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stderr.String(), gocheck.Equals, "Opening config file: testdata/tsuru.conf\nDone reading config file: testdata/tsuru.conf\n")
	c.Assert(stdout.String(), gocheck.Equals, "")
	c.Assert(fakeCmd.Calls(), gocheck.Equals, int32(1))
}
