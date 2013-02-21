// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"errors"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/fs/testing"
	"io"
	. "launchpad.net/gocheck"
	"os"
)

type recordingExiter int

func (e *recordingExiter) Exit(code int) {
	*e = recordingExiter(code)
}

func (e recordingExiter) value() int {
	return int(e)
}

type TestCommand struct{}

func (c *TestCommand) Info() *Info {
	return &Info{
		Name:  "foo",
		Desc:  "Foo do anything or nothing.",
		Usage: "foo",
	}
}

func (c *TestCommand) Run(context *Context, client Doer) error {
	io.WriteString(context.Stdout, "Running TestCommand")
	return nil
}

type ErrorCommand struct {
	msg string
}

func (c *ErrorCommand) Info() *Info {
	return &Info{Name: "error"}
}

func (c *ErrorCommand) Run(context *Context, client Doer) error {
	return errors.New(c.msg)
}

func (s *S) TestRegister(c *C) {
	manager.Register(&TestCommand{})
	badCall := func() { manager.Register(&TestCommand{}) }
	c.Assert(badCall, PanicMatches, "command already registered: foo")
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldReturnStatus1WhenCommandFail(c *C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.e.(*recordingExiter).value(), Equals, 1)
}

func (s *S) TestManagerRunShouldAppendNewLineOnErrorWhenItsNotPresent(c *C) {
	manager.Register(&ErrorCommand{msg: "You are wrong"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), Equals, "Error: You are wrong\n")
}

func (s *S) TestRun(c *C) {
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), Equals, `Error: command "bar" does not exist`+"\n")
	c.Assert(manager.e.(*recordingExiter).value(), Equals, 1)
}
func (s *S) TestHelp(c *C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help
  user-create
  version

Run glb help <commandname> to get more information about a specific command.
`
	manager.Register(&userCreate{})
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestHelpCommandShouldBeRegisteredByDefault(c *C) {
	var stdout, stderr bytes.Buffer
	m := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin)
	_, exists := m.Commands["help"]
	c.Assert(exists, Equals, true)
}

func (s *S) TestHelpReturnErrorIfTheGivenCommandDoesNotExist(c *C) {
	command := help{manager: manager}
	context := Context{[]string{"user-create"}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, `^Error: command "user-create" does not exist.$`)
}

func (s *S) TestRunWithoutArgsShouldRunsHelp(c *C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help
  version

Run glb help <commandname> to get more information about a specific command.
`
	manager.Run([]string{})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestHelpShouldReturnHelpForACmd(c *C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.
`
	manager.Register(&TestCommand{})
	manager.Run([]string{"help", "foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestVersion(c *C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "5.0", "", &stdout, &stderr, os.Stdin)
	command := version{manager: manager}
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, "tsuru version 5.0.\n")
}

func (s *S) TestVersionInfo(c *C) {
	expected := &Info{
		Name:    "version",
		MinArgs: 0,
		Usage:   "version",
		Desc:    "display the current version",
	}
	c.Assert((&version{}).Info(), DeepEquals, expected)
}

type ArgCmd struct{}

func (c *ArgCmd) Info() *Info {
	return &Info{
		Name:    "arg",
		MinArgs: 1,
		Usage:   "arg [args]",
		Desc:    "some desc",
	}
}

func (cmd *ArgCmd) Run(ctx *Context, client Doer) error {
	return nil
}

func (s *S) TestRunWrongArgsNumberShouldRunsHelpAndReturnStatus1(c *C) {
	expected := `glb version 1.0.

ERROR: not enough arguments to call arg.

Usage: glb arg [args]

some desc

Minimum arguments: 1
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
	c.Assert(manager.e.(*recordingExiter).value(), Equals, 1)
}

func (s *S) TestHelpShouldReturnUsageWithTheCommandName(c *C) {
	expected := `tsuru version 1.0.

Usage: tsuru foo

Foo do anything or nothing.
`
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin)
	manager.Register(&TestCommand{})
	context := Context{[]string{"foo"}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestExtractProgramNameWithAbsolutePath(c *C) {
	got := ExtractProgramName("/usr/bin/tsuru")
	c.Assert(got, Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithRelativePath(c *C) {
	got := ExtractProgramName("./tsuru")
	c.Assert(got, Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithinThePATH(c *C) {
	got := ExtractProgramName("tsuru")
	c.Assert(got, Equals, "tsuru")
}

func (s *S) TestFinisherReturnsOsExiterIfNotDefined(c *C) {
	m := Manager{}
	c.Assert(m.finisher(), FitsTypeOf, osExiter{})
}

func (s *S) TestFinisherReturnTheDefinedE(c *C) {
	var exiter recordingExiter
	m := Manager{e: &exiter}
	c.Assert(m.finisher(), FitsTypeOf, &exiter)
}

func (s *S) TestLoginIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	lgn, ok := manager.Commands["login"]
	c.Assert(ok, Equals, true)
	c.Assert(lgn, FitsTypeOf, &login{})
}

func (s *S) TestLogoutIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	lgt, ok := manager.Commands["logout"]
	c.Assert(ok, Equals, true)
	c.Assert(lgt, FitsTypeOf, &logout{})
}

func (s *S) TestUserCreateIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	user, ok := manager.Commands["user-create"]
	c.Assert(ok, Equals, true)
	c.Assert(user, FitsTypeOf, &userCreate{})
}

func (s *S) TestTeamCreateIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	create, ok := manager.Commands["team-create"]
	c.Assert(ok, Equals, true)
	c.Assert(create, FitsTypeOf, &teamCreate{})
}

func (s *S) TestTeamListIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	list, ok := manager.Commands["team-list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &teamList{})
}

func (s *S) TestTeamAddUserIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	adduser, ok := manager.Commands["team-user-add"]
	c.Assert(ok, Equals, true)
	c.Assert(adduser, FitsTypeOf, &teamUserAdd{})
}

func (s *S) TestTeamRemoveUserIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	removeuser, ok := manager.Commands["team-user-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(removeuser, FitsTypeOf, &teamUserRemove{})
}

func (s *S) TestTargetIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	tgt, ok := manager.Commands["target"]
	c.Assert(ok, Equals, true)
	c.Assert(tgt, FitsTypeOf, &target{})
}

func (s *S) TestUserRemoveIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	rmUser, ok := manager.Commands["user-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(rmUser, FitsTypeOf, &userRemove{})
}

func (s *S) TestTeamRemoveIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	rmTeam, ok := manager.Commands["team-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(rmTeam, FitsTypeOf, &teamRemove{})
}

func (s *S) TestChangePasswordIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru", "1.0", "")
	chpass, ok := manager.Commands["change-password"]
	c.Assert(ok, Equals, true)
	c.Assert(chpass, FitsTypeOf, &changePassword{})
}

func (s *S) TestVersionIsRegisteredByNewManager(c *C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin)
	ver, ok := manager.Commands["version"]
	c.Assert(ok, Equals, true)
	c.Assert(ver, FitsTypeOf, &version{})
}

func (s *S) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
}

func (s *S) TestValidateVersion(c *C) {
	var cases = []struct {
		current, support string
		expected         bool
	}{
		{
			current:  "0.2.1",
			support:  "0.3",
			expected: false,
		},
		{
			current:  "0.3.5",
			support:  "0.3",
			expected: true,
		},
		{
			current:  "0.2",
			support:  "0.3",
			expected: false,
		},
	}
	for _, cs := range cases {
		c.Assert(validateVersion(cs.support, cs.current), Equals, cs.expected)
	}
}
