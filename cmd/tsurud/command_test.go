// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
	check "gopkg.in/check.v1"
)

func (s *S) TestConfigFileValueString(c *check.C) {
	value := "/etc/tsuru/tsuru.conf"
	file := configFile{value: value}
	c.Assert(file.String(), check.Equals, value)
}

func (s *S) TestConfigFileValueSet(c *check.C) {
	var file configFile
	err := file.Set("testdata/tsuru2.conf")
	c.Assert(err, check.IsNil)
	c.Assert(file.String(), check.Equals, "testdata/tsuru2.conf")
	c.Assert(configPath, check.Equals, "testdata/tsuru2.conf")
}

func (s *S) TestTsrCommandFlagged(c *check.C) {
	var originalCmd fakeCommand
	cmd := tsurudCommand{Command: &originalCmd}
	flags := cmd.Flags()
	err := flags.Parse(true, []string{"--name", "Chico", "--config", "testdata/tsuru.conf"})
	c.Assert(err, check.IsNil)
	c.Assert(originalCmd.name, check.Equals, "Chico")
	c.Assert(cmd.file.String(), check.Equals, "testdata/tsuru.conf")
	cmd = tsurudCommand{Command: &originalCmd}
	flags.Parse(true, []string{"--name", "Chico", "-c", "testdata/tsuru.conf"})
	c.Assert(cmd.file.String(), check.Equals, "testdata/tsuru.conf")
}

type fakeCommand struct {
	name string
	fs   *gnuflag.FlagSet
}

func (f *fakeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "fake",
		Usage:   "fake",
		Desc:    `Fake`,
		MinArgs: 1,
	}
}

func (f *fakeCommand) Run(*cmd.Context) error {
	return nil
}

func (f *fakeCommand) Flags() *gnuflag.FlagSet {
	if f.fs == nil {
		f.fs = gnuflag.NewFlagSet("fakeCommand", gnuflag.ExitOnError)
		f.fs.StringVar(&f.name, "name", "", "your name")
	}
	return f.fs
}

func (s *S) TestTsrCommandRunInvalidConfig(c *check.C) {
	fakeCmd := FakeCommand{}
	command := tsurudCommand{Command: &fakeCmd}
	command.Flags().Parse(true, []string{"-c", "/invalid/file"})
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := command.Run(&context)
	c.Assert(err, check.NotNil)
	c.Assert(stderr.String(), check.Equals, `Opening config file: /invalid/file
Could not open tsuru config file at /invalid/file (open /invalid/file: no such file or directory).
  For an example, see: tsuru/etc/tsuru.conf
  Note that you can specify a different config file with the --config option -- e.g.: --config=./etc/tsuru.conf
`)
	c.Assert(stdout.String(), check.Equals, "")
	c.Assert(fakeCmd.Calls(), check.Equals, int32(0))
}

func (s *S) TestTsrCommandRun(c *check.C) {
	fakeCmd := FakeCommand{}
	command := tsurudCommand{Command: &fakeCmd}
	command.Flags().Parse(true, []string{"-c", "testdata/tsuru.conf"})
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := command.Run(&context)
	c.Assert(err, check.IsNil)
	c.Assert(stderr.String(), check.Equals, "Opening config file: testdata/tsuru.conf\nDone reading config file: testdata/tsuru.conf\n")
	c.Assert(stdout.String(), check.Equals, "")
	c.Assert(fakeCmd.Calls(), check.Equals, int32(1))
}
