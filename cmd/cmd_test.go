// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/fs/testing"
	"launchpad.net/gnuflag"
	"launchpad.net/gocheck"
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

func (c *TestCommand) Run(context *Context, client *Client) error {
	io.WriteString(context.Stdout, "Running TestCommand")
	return nil
}

type TestNamedCommand struct {
	TestCommand
}

func (c *TestNamedCommand) Name() string {
	return "bar"
}


type ErrorCommand struct {
	msg string
}

func (c *ErrorCommand) Info() *Info {
	return &Info{Name: "error"}
}

func (c *ErrorCommand) Run(context *Context, client *Client) error {
	if c.msg == "abort" {
		return ErrAbortCommand
	}
	return errors.New(c.msg)
}

type UnauthorizedErrorCommand struct{}

func (c *UnauthorizedErrorCommand) Info() *Info {
	return &Info{Name: "unauthorized-error"}
}

func (c *UnauthorizedErrorCommand) Run(context *Context, client *Client) error {
	return &tsuruErrors.HTTP{Code: http.StatusUnauthorized, Message: "my error"}
}

type UnauthorizedLoginErrorCommand struct {
	UnauthorizedErrorCommand
}

func (c *UnauthorizedLoginErrorCommand) Info() *Info {
	return &Info{Name: "login"}
}

type CommandWithFlags struct {
	fs      *gnuflag.FlagSet
	age     int
	minArgs int
	args    []string
}

func (c *CommandWithFlags) Info() *Info {
	return &Info{Name: "with-flags", MinArgs: c.minArgs}
}

func (c *CommandWithFlags) Run(context *Context, client *Client) error {
	c.args = context.Args
	return nil
}

func (c *CommandWithFlags) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.IntVar(&c.age, "age", 0, "your age")
	}
	return c.fs
}

func (s *S) TestDeprecatedCommand(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	cmd := TestCommand{}
	manager.RegisterDeprecated(&cmd, "bar")
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"bar"})
	c.Assert(stdout.String(), gocheck.Equals, "Running TestCommand")
	warnMessage := `WARNING: "bar" has been deprecated, please use "foo" instead.` + "\n\n"
	c.Assert(stderr.String(), gocheck.Equals, warnMessage)
	stdout.Reset()
	stderr.Reset()
	manager.Run([]string{"foo"})
	c.Assert(stdout.String(), gocheck.Equals, "Running TestCommand")
	c.Assert(stderr.String(), gocheck.Equals, "")
}

func (s *S) TestDeprecatedCommandFlags(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	cmd := CommandWithFlags{}
	manager.RegisterDeprecated(&cmd, "bar")
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"bar", "--age", "10"})
	warnMessage := `WARNING: "bar" has been deprecated, please use "with-flags" instead.` + "\n\n"
	c.Assert(stderr.String(), gocheck.Equals, warnMessage)
	c.Assert(cmd.age, gocheck.Equals, 10)
}

func (s *S) TestRegister(c *gocheck.C) {
	manager.Register(&TestCommand{})
	badCall := func() { manager.Register(&TestCommand{}) }
	c.Assert(badCall, gocheck.PanicMatches, "command already registered: foo")
}

func (s *S) TestRegisterDeprecated(c *gocheck.C) {
	originalCmd := &TestCommand{}
	manager.RegisterDeprecated(originalCmd, "bar")
	badCall := func() { manager.Register(originalCmd) }
	c.Assert(badCall, gocheck.PanicMatches, "command already registered: foo")
	cmd, ok := manager.Commands["bar"].(*DeprecatedCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(cmd.Command, gocheck.Equals, originalCmd)
	c.Assert(manager.Commands["foo"], gocheck.Equals, originalCmd)
}

func (s *S) TestRegisterNamed(c *gocheck.C) {
	manager.Register(&TestNamedCommand{})
	badCall := func() { manager.Register(&TestNamedCommand{}) }
	c.Assert(badCall, gocheck.PanicMatches, "command already registered: bar")
}

func (s *S) TestRegisterDeprecatedName(c *gocheck.C) {
	originalCmd := &TestNamedCommand{}
	manager.RegisterDeprecated(originalCmd, "foo")
	badCall := func() { manager.Register(originalCmd) }
	c.Assert(badCall, gocheck.PanicMatches, "command already registered: bar")
	cmd, ok := manager.Commands["foo"].(*DeprecatedCommand)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(cmd.Command, gocheck.Equals, originalCmd)
}

func (s *S) TestRegisterTopic(c *gocheck.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetting everything!")
	c.Assert(manager.topics["target"], gocheck.Equals, "targetting everything!")
}

func (s *S) TestRegisterTopicDuplicated(c *gocheck.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetting everything!")
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	manager.RegisterTopic("target", "wat")
}

func (s *S) TestRegisterTopicMultiple(c *gocheck.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetted")
	manager.RegisterTopic("app", "what's an app?")
	expected := map[string]string{
		"target": "targetted",
		"app":    "what's an app?",
	}
	c.Assert(manager.topics, gocheck.DeepEquals, expected)
}

func (s *S) TestCustomLookup(c *gocheck.C) {
	lookup := func(ctx *Context) error {
		fmt.Fprintf(ctx.Stdout, "test")
		return nil
	}
	var stdout, stderr bytes.Buffer
	manager := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	manager.Run([]string{"custom"})
	c.Assert(stdout.String(), gocheck.Equals, "test")
}

func (s *S) TestCustomLookupNotFound(c *gocheck.C) {
	lookup := func(ctx *Context) error {
		return os.ErrNotExist
	}
	var stdout, stderr bytes.Buffer
	manager := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	var exiter recordingExiter
	manager.e = &exiter
	manager.Run([]string{"custom"})
	c.Assert(stderr.String(), gocheck.Equals, "Error: command \"custom\" does not exist\n")
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *gocheck.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldReturnStatus1WhenCommandFail(c *gocheck.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestManagerRunShouldAppendNewLineOnErrorWhenItsNotPresent(c *gocheck.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldNotWriteErrorOnStderrWhenErrAbortIsTriggered(c *gocheck.C) {
	manager.Register(&ErrorCommand{msg: "abort"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, "")
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestManagerRunWithHTTPUnauthorizedError(c *gocheck.C) {
	manager.Register(&UnauthorizedErrorCommand{})
	manager.Run([]string{"unauthorized-error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, `Error: You're not authenticated or your session has expired. Please use "login" command for authentication.`+"\n")
}

func (s *S) TestManagerRunLoginWithHTTPUnauthorizedError(c *gocheck.C) {
	manager.Register(&UnauthorizedLoginErrorCommand{})
	manager.Run([]string{"login"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, "Error: my error\n")
}

func (s *S) TestManagerRunWithFlags(c *gocheck.C) {
	cmd := &CommandWithFlags{}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "--age", "10"})
	c.Assert(cmd.fs.Parsed(), gocheck.Equals, true)
	c.Assert(cmd.age, gocheck.Equals, 10)
}

func (s *S) TestManagerRunWithFlagsAndArgs(c *gocheck.C) {
	cmd := &CommandWithFlags{minArgs: 2}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "something", "--age", "20", "otherthing"})
	c.Assert(cmd.args, gocheck.DeepEquals, []string{"something", "otherthing"})
}

func (s *S) TestManagerRunWithInvalidValueForFlag(c *gocheck.C) {
	var exiter recordingExiter
	old := manager.e
	manager.e = &exiter
	defer func() {
		manager.e = old
	}()
	cmd := &CommandWithFlags{}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "--age", "tsuru"})
	c.Assert(cmd.fs.Parsed(), gocheck.Equals, true)
	c.Assert(exiter.value(), gocheck.Equals, 1)
}

func (s *S) TestRun(c *gocheck.C) {
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *gocheck.C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), gocheck.Equals, `Error: command "bar" does not exist`+"\n")
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestHelp(c *gocheck.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help
  user-create
  version

Use glb help <commandname> to get more information about a command.
`
	manager.RegisterDeprecated(&userCreate{}, "create-user")
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestHelpWithTopics(c *gocheck.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help
  user-create
  version

Use glb help <commandname> to get more information about a command.

Available topics:
  target

Use glb help <topicname> to get more information about a topic.
`
	manager.Register(&userCreate{})
	manager.RegisterTopic("target", "something")
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestHelpFromTopic(c *gocheck.C) {
	expected := `glb version 1.0.

Targets

Tsuru likes to manage targets
`
	manager.RegisterTopic("target", "Targets\n\nTsuru likes to manage targets\n")
	context := Context{[]string{"target"}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestHelpCommandShouldBeRegisteredByDefault(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	m := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	_, exists := m.Commands["help"]
	c.Assert(exists, gocheck.Equals, true)
}

func (s *S) TestHelpReturnErrorIfTheGivenCommandDoesNotExist(c *gocheck.C) {
	command := help{manager: manager}
	context := Context{[]string{"user-create"}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, `^command "user-create" does not exist.$`)
}

func (s *S) TestRunWithoutArgsShouldRunHelp(c *gocheck.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help
  version

Use glb help <commandname> to get more information about a command.
`
	manager.Run([]string{})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestHelpShouldReturnHelpForACmd(c *gocheck.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	manager.Register(&TestCommand{})
	manager.Run([]string{"help", "foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestHelpDeprecatedCmd(c *gocheck.C) {
	expectedStdout := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	expectedStderr := `WARNING: "bar" is deprecated. Showing help for "foo" instead.` + "\n\n"
	var stdout, stderr bytes.Buffer
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.RegisterDeprecated(&TestCommand{}, "bar")
	manager.Run([]string{"help", "bar"})
	c.Assert(stdout.String(), gocheck.Equals, expectedStdout)
	c.Assert(stderr.String(), gocheck.Equals, expectedStderr)
	stdout.Reset()
	stderr.Reset()
	manager.Run([]string{"help", "foo"})
	c.Assert(stdout.String(), gocheck.Equals, expectedStdout)
	c.Assert(stderr.String(), gocheck.Equals, "")
}

func (s *S) TestHelpDeprecatedCmdWritesWarningFirst(c *gocheck.C) {
	expected := `WARNING: "bar" is deprecated. Showing help for "foo" instead.

glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	var output bytes.Buffer
	manager.stdout = &output
	manager.stderr = &output
	manager.RegisterDeprecated(&TestCommand{}, "bar")
	manager.Run([]string{"help", "bar"})
	c.Assert(output.String(), gocheck.Equals, expected)
}

func (s *S) TestVersion(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "5.0", "", &stdout, &stderr, os.Stdin, nil)
	command := version{manager: manager}
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, "tsuru version 5.0.\n")
}

func (s *S) TestVersionInfo(c *gocheck.C) {
	expected := &Info{
		Name:    "version",
		MinArgs: 0,
		Usage:   "version",
		Desc:    "display the current version",
	}
	c.Assert((&version{}).Info(), gocheck.DeepEquals, expected)
}

type ArgCmd struct{}

func (c *ArgCmd) Info() *Info {
	return &Info{
		Name:    "arg",
		MinArgs: 1,
		MaxArgs: 2,
		Usage:   "arg [args]",
		Desc:    "some desc",
	}
}

func (cmd *ArgCmd) Run(ctx *Context, client *Client) error {
	return nil
}

func (s *S) TestRunWrongArgsNumberShouldRunsHelpAndReturnStatus1(c *gocheck.C) {
	expected := `glb version 1.0.

ERROR: wrong number of arguments.

Usage: glb arg [args]

some desc

Minimum # of arguments: 1
Maximum # of arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestRunWithTooManyArguments(c *gocheck.C) {
	expected := `glb version 1.0.

ERROR: wrong number of arguments.

Usage: glb arg [args]

some desc

Minimum # of arguments: 1
Maximum # of arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg", "param1", "param2", "param3"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
	c.Assert(manager.e.(*recordingExiter).value(), gocheck.Equals, 1)
}

func (s *S) TestHelpShouldReturnUsageWithTheCommandName(c *gocheck.C) {
	expected := `tsuru version 1.0.

Usage: tsuru foo

Foo do anything or nothing.

`
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	manager.Register(&TestCommand{})
	context := Context{[]string{"foo"}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), gocheck.Equals, expected)
}

func (s *S) TestExtractProgramNameWithAbsolutePath(c *gocheck.C) {
	got := ExtractProgramName("/usr/bin/tsuru")
	c.Assert(got, gocheck.Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithRelativePath(c *gocheck.C) {
	got := ExtractProgramName("./tsuru")
	c.Assert(got, gocheck.Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithinThePATH(c *gocheck.C) {
	got := ExtractProgramName("tsuru")
	c.Assert(got, gocheck.Equals, "tsuru")
}

func (s *S) TestFinisherReturnsOsExiterIfNotDefined(c *gocheck.C) {
	m := Manager{}
	c.Assert(m.finisher(), gocheck.FitsTypeOf, osExiter{})
}

func (s *S) TestFinisherReturnTheDefinedE(c *gocheck.C) {
	var exiter recordingExiter
	m := Manager{e: &exiter}
	c.Assert(m.finisher(), gocheck.FitsTypeOf, &exiter)
}

func (s *S) TestLoginIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	lgn, ok := manager.Commands["login"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(lgn, gocheck.FitsTypeOf, &login{})
}

func (s *S) TestLogoutIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	lgt, ok := manager.Commands["logout"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(lgt, gocheck.FitsTypeOf, &logout{})
}

func (s *S) TestUserCreateIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	user, ok := manager.Commands["user-create"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(user, gocheck.FitsTypeOf, &userCreate{})
}

func (s *S) TestTeamCreateIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	create, ok := manager.Commands["team-create"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(create, gocheck.FitsTypeOf, &teamCreate{})
}

func (s *S) TestTeamListIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	list, ok := manager.Commands["team-list"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(list, gocheck.FitsTypeOf, &teamList{})
}

func (s *S) TestTeamAddUserIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	adduser, ok := manager.Commands["team-user-add"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(adduser, gocheck.FitsTypeOf, &teamUserAdd{})
}

func (s *S) TestTeamRemoveUserIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	removeuser, ok := manager.Commands["team-user-remove"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(removeuser, gocheck.FitsTypeOf, &teamUserRemove{})
}

func (s *S) TestTeamUserListIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	listuser, ok := manager.Commands["team-user-list"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(listuser, gocheck.FitsTypeOf, teamUserList{})
}

func (s *S) TestTargetListIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	tgt, ok := manager.Commands["target-list"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(tgt, gocheck.FitsTypeOf, &targetList{})
}

func (s *S) TestTargetTopicIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	expected := fmt.Sprintf(targetTopic, "tsuru")
	c.Assert(manager.topics["target"], gocheck.Equals, expected)
}

func (s *S) TestUserRemoveIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	rmUser, ok := manager.Commands["user-remove"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(rmUser, gocheck.FitsTypeOf, &userRemove{})
}

func (s *S) TestTeamRemoveIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	rmTeam, ok := manager.Commands["team-remove"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(rmTeam, gocheck.FitsTypeOf, &teamRemove{})
}

func (s *S) TestChangePasswordIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	chpass, ok := manager.Commands["change-password"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(chpass, gocheck.FitsTypeOf, &changePassword{})
}

func (s *S) TestResetPasswordIsRegistered(c *gocheck.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	reset, ok := manager.Commands["reset-password"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(reset, gocheck.FitsTypeOf, &resetPassword{})
}

func (s *S) TestVersionIsRegisteredByNewManager(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	ver, ok := manager.Commands["version"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(ver, gocheck.FitsTypeOf, &version{})
}

func (s *S) TestFileSystem(c *gocheck.C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), gocheck.DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), gocheck.DeepEquals, fs.OsFs{})
}

func (s *S) TestValidateVersion(c *gocheck.C) {
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
		{
			current:  "0.7.10",
			support:  "0.7.2",
			expected: true,
		},
		{
			current:  "beta",
			support:  "0.7.2",
			expected: false,
		},
		{
			current:  "0.7.10",
			support:  "beta",
			expected: false,
		},
		{
			current:  "0.7.10",
			support:  "",
			expected: true,
		},
		{
			current:  "0.8",
			support:  "0.7.15",
			expected: true,
		},
		{
			current:  "0.8",
			support:  "0.8",
			expected: true,
		},
	}
	for _, cs := range cases {
		c.Check(validateVersion(cs.support, cs.current), gocheck.Equals, cs.expected)
	}
}
