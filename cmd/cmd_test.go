// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
	"launchpad.net/gnuflag"
)

func (s *S) TestDeprecatedCommand(c *check.C) {
	var stdout, stderr bytes.Buffer
	cmd := TestCommand{}
	manager.RegisterDeprecated(&cmd, "bar")
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"bar"})
	c.Assert(stdout.String(), check.Equals, "Running TestCommand")
	warnMessage := `WARNING: "bar" has been deprecated, please use "foo" instead.` + "\n\n"
	c.Assert(stderr.String(), check.Equals, warnMessage)
	stdout.Reset()
	stderr.Reset()
	manager.Run([]string{"foo"})
	c.Assert(stdout.String(), check.Equals, "Running TestCommand")
	c.Assert(stderr.String(), check.Equals, "")
}

func (s *S) TestDeprecatedCommandFlags(c *check.C) {
	var stdout, stderr bytes.Buffer
	cmd := CommandWithFlags{}
	manager.RegisterDeprecated(&cmd, "bar")
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"bar", "--age", "10"})
	warnMessage := `WARNING: "bar" has been deprecated, please use "with-flags" instead.` + "\n\n"
	c.Assert(stderr.String(), check.Equals, warnMessage)
	c.Assert(cmd.age, check.Equals, 10)
}

func (s *S) TestRegister(c *check.C) {
	manager.Register(&TestCommand{})
	badCall := func() { manager.Register(&TestCommand{}) }
	c.Assert(badCall, check.PanicMatches, "command already registered: foo")
}

func (s *S) TestRegisterDeprecated(c *check.C) {
	originalCmd := &TestCommand{}
	manager.RegisterDeprecated(originalCmd, "bar")
	badCall := func() { manager.Register(originalCmd) }
	c.Assert(badCall, check.PanicMatches, "command already registered: foo")
	cmd, ok := manager.Commands["bar"].(*DeprecatedCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(cmd.Command, check.Equals, originalCmd)
	c.Assert(manager.Commands["foo"], check.Equals, originalCmd)
}

func (s *S) TestRegisterTopic(c *check.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetting everything!")
	c.Assert(manager.topics["target"], check.Equals, "targetting everything!")
}

func (s *S) TestRegisterTopicDuplicated(c *check.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetting everything!")
	defer func() {
		r := recover()
		c.Assert(r, check.NotNil)
	}()
	manager.RegisterTopic("target", "wat")
}

func (s *S) TestRegisterTopicMultiple(c *check.C) {
	manager := Manager{}
	manager.RegisterTopic("target", "targetted")
	manager.RegisterTopic("app", "what's an app?")
	expected := map[string]string{
		"target": "targetted",
		"app":    "what's an app?",
	}
	c.Assert(manager.topics, check.DeepEquals, expected)
}

func (s *S) TestCustomLookup(c *check.C) {
	lookup := func(ctx *Context) error {
		fmt.Fprintf(ctx.Stdout, "test")
		return nil
	}
	var stdout, stderr bytes.Buffer
	manager := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	manager.Run([]string{"custom"})
	c.Assert(stdout.String(), check.Equals, "test")
}

func (s *S) TestCustomLookupNotFound(c *check.C) {
	lookup := func(ctx *Context) error {
		return os.ErrNotExist
	}
	var stdout, stderr bytes.Buffer
	manager := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	var exiter recordingExiter
	manager.e = &exiter
	manager.Run([]string{"custom"})
	c.Assert(strings.Replace(stderr.String(), "\n", "\\n", -1), check.Matches, `.*: "custom" is not a tsuru command. See "tsuru help".*`)
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *check.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldReturnStatus1WhenCommandFail(c *check.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestManagerRunShouldAppendNewLineOnErrorWhenItsNotPresent(c *check.C) {
	manager.Register(&ErrorCommand{msg: "You are wrong"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldNotWriteErrorOnStderrWhenErrAbortIsTriggered(c *check.C) {
	manager.Register(&ErrorCommand{msg: "abort"})
	manager.Run([]string{"error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, "")
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestManagerRunWithHTTPUnauthorizedError(c *check.C) {
	manager.Register(&UnauthorizedErrorCommand{})
	manager.Run([]string{"unauthorized-error"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, `Error: You're not authenticated or your session has expired. Please use "login" command for authentication.`+"\n")
}

func (s *S) TestManagerRunWithHTTPUnauthorizedErrorAndLoginRegistered(c *check.C) {
	expectedStderr := `Error: you're not authenticated or your session has expired.
Calling the "login" command...

`
	expectedStdout := `logged in!
worked nicely!
`
	manager.Register(&FailAndWorkCommand{})
	manager.Register(&SuccessLoginCommand{})
	manager.Run([]string{"fail-and-work"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, expectedStderr)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expectedStdout)
}

func (s *S) TestManagerRunWithHTTPUnauthorizedErrorAndLoginFailure(c *check.C) {
	expected := `Error: you're not authenticated or your session has expired.
Calling the "login" command...
Error: You're not authenticated or your session has expired. Please use "login" command for authentication.
`
	manager.Register(&FailAndWorkCommand{})
	manager.Register(&UnauthorizedLoginErrorCommand{})
	manager.Run([]string{"fail-and-work"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestManagerRunLoginWithHTTPUnauthorizedError(c *check.C) {
	manager.Register(&UnauthorizedLoginErrorCommand{})
	manager.Run([]string{"login"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: unauthorized\n")
}

func (s *S) TestManagerRunWithFlags(c *check.C) {
	cmd := &CommandWithFlags{}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "--age", "10"})
	c.Assert(cmd.fs.Parsed(), check.Equals, true)
	c.Assert(cmd.age, check.Equals, 10)
}

func (s *S) TestManagerRunWithFlagsAndArgs(c *check.C) {
	cmd := &CommandWithFlags{minArgs: 2}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "something", "--age", "20", "otherthing"})
	c.Assert(cmd.args, check.DeepEquals, []string{"something", "otherthing"})
}

func (s *S) TestManagerRunWithInvalidValueForFlag(c *check.C) {
	var exiter recordingExiter
	old := manager.e
	manager.e = &exiter
	defer func() {
		manager.e = old
	}()
	cmd := &CommandWithFlags{}
	manager.Register(cmd)
	manager.Run([]string{"with-flags", "--age", "tsuru"})
	c.Assert(cmd.fs.Parsed(), check.Equals, true)
	c.Assert(exiter.value(), check.Equals, 1)
}

func (s *S) TestRun(c *check.C) {
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *check.C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.stderr.(*bytes.Buffer).String(), check.Equals, `Error: command "bar" does not exist`+"\n")
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  version              Display the current version

Use glb help <commandname> to get more information about a command.
`
	manager.RegisterDeprecated(&login{}, "login")
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpWithTopics(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  login                Initiates a new tsuru session for a user
  version              Display the current version

Use glb help <commandname> to get more information about a command.

Available topics:
  target

Use glb help <topicname> to get more information about a topic.
`
	manager.Register(&login{})
	manager.RegisterTopic("target", "something")
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpFromTopic(c *check.C) {
	expected := `glb version 1.0.

Targets

Tsuru likes to manage targets
`
	manager.RegisterTopic("target", "Targets\n\nTsuru likes to manage targets\n")
	context := Context{[]string{"target"}, manager.stdout, manager.stderr, manager.stdin}
	command := help{manager: manager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpCommandShouldBeRegisteredByDefault(c *check.C) {
	var stdout, stderr bytes.Buffer
	m := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	_, exists := m.Commands["help"]
	c.Assert(exists, check.Equals, true)
}

func (s *S) TestHelpReturnErrorIfTheGivenCommandDoesNotExist(c *check.C) {
	command := help{manager: manager}
	context := Context{[]string{"user-create"}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `^command "user-create" does not exist.$`)
}

func (s *S) TestRunWithoutArgsShouldRunHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  version              Display the current version

Use glb help <commandname> to get more information about a command.
`
	manager.Run([]string{})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDashDashHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  version              Display the current version

Use glb help <commandname> to get more information about a command.
`
	manager.Run([]string{"--help"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestRunCommandWithDashHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo", "--help"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestRunCommandWithDashH(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo", "-h"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpShouldReturnHelpForACmd(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	manager.Register(&TestCommand{})
	manager.Run([]string{"help", "foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDashDashHelpShouldReturnHelpForACmd(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	manager.Register(&TestCommand{})
	manager.Run([]string{"--help", "foo"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDuplicateHelpFlag(c *check.C) {
	expected := "help called? true"
	manager.Register(&HelpCommandWithFlags{})
	manager.Run([]string{"hflags", "--help"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDuplicateHFlag(c *check.C) {
	expected := "help called? true"
	manager.Register(&HelpCommandWithFlags{})
	manager.Run([]string{"hflags", "-h"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpFlaggedCommand(c *check.C) {
	expected := `glb version 1.0.

Usage: glb with-flags

with-flags doesn't do anything, really.

Flags:
  
  -a, --age  (= 0)
      your age
  
`
	manager.Register(&CommandWithFlags{})
	manager.Run([]string{"help", "with-flags"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpDeprecatedCmd(c *check.C) {
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
	c.Assert(stdout.String(), check.Equals, expectedStdout)
	c.Assert(stderr.String(), check.Equals, expectedStderr)
	stdout.Reset()
	stderr.Reset()
	manager.Run([]string{"help", "foo"})
	c.Assert(stdout.String(), check.Equals, expectedStdout)
	c.Assert(stderr.String(), check.Equals, "")
}

func (s *S) TestHelpDeprecatedCmdWritesWarningFirst(c *check.C) {
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
	c.Assert(output.String(), check.Equals, expected)
}

func (s *S) TestVersion(c *check.C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "5.0", "", &stdout, &stderr, os.Stdin, nil)
	command := version{manager: manager}
	context := Context{[]string{}, manager.stdout, manager.stderr, manager.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, "tsuru version 5.0.\n")
}

func (s *S) TestDashDashVersion(c *check.C) {
	expected := "glb version 1.0.\n"
	manager.Run([]string{"--version"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestVersionInfo(c *check.C) {
	expected := &Info{
		Name:    "version",
		MinArgs: 0,
		Usage:   "version",
		Desc:    "display the current version",
	}
	c.Assert((&version{}).Info(), check.DeepEquals, expected)
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

func (s *S) TestRunWrongArgsNumberShouldRunsHelpAndReturnStatus1(c *check.C) {
	expected := `glb version 1.0.

ERROR: wrong number of arguments.

Usage: glb arg [args]

some desc

Minimum # of arguments: 1
Maximum # of arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestRunWithTooManyArguments(c *check.C) {
	expected := `glb version 1.0.

ERROR: wrong number of arguments.

Usage: glb arg [args]

some desc

Minimum # of arguments: 1
Maximum # of arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg", "param1", "param2", "param3"})
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestHelpShouldReturnUsageWithTheCommandName(c *check.C) {
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
	c.Assert(err, check.IsNil)
	c.Assert(manager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestExtractProgramNameWithAbsolutePath(c *check.C) {
	got := ExtractProgramName("/usr/bin/tsuru")
	c.Assert(got, check.Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithRelativePath(c *check.C) {
	got := ExtractProgramName("./tsuru")
	c.Assert(got, check.Equals, "tsuru")
}

func (s *S) TestExtractProgramNameWithinThePATH(c *check.C) {
	got := ExtractProgramName("tsuru")
	c.Assert(got, check.Equals, "tsuru")
}

func (s *S) TestFinisherReturnsOsExiterIfNotDefined(c *check.C) {
	m := Manager{}
	c.Assert(m.finisher(), check.FitsTypeOf, osExiter{})
}

func (s *S) TestFinisherReturnTheDefinedE(c *check.C) {
	var exiter recordingExiter
	m := Manager{e: &exiter}
	c.Assert(m.finisher(), check.FitsTypeOf, &exiter)
}

func (s *S) TestLoginIsRegistered(c *check.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	lgn, ok := manager.Commands["login"]
	c.Assert(ok, check.Equals, true)
	c.Assert(lgn, check.FitsTypeOf, &login{})
}

func (s *S) TestLogoutIsRegistered(c *check.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	lgt, ok := manager.Commands["logout"]
	c.Assert(ok, check.Equals, true)
	c.Assert(lgt, check.FitsTypeOf, &logout{})
}

func (s *S) TestTargetListIsRegistered(c *check.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	tgt, ok := manager.Commands["target-list"]
	c.Assert(ok, check.Equals, true)
	c.Assert(tgt, check.FitsTypeOf, &targetList{})
}

func (s *S) TestTargetTopicIsRegistered(c *check.C) {
	manager := BuildBaseManager("tsuru", "1.0", "", nil)
	expected := fmt.Sprintf(targetTopic, "tsuru")
	c.Assert(manager.topics["target"], check.Equals, expected)
}

func (s *S) TestVersionIsRegisteredByNewManager(c *check.C) {
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	ver, ok := manager.Commands["version"]
	c.Assert(ok, check.Equals, true)
	c.Assert(ver, check.FitsTypeOf, &version{})
}

func (s *S) TestInvalidCommandFuzzyMatch01(c *check.C) {
	lookup := func(ctx *Context) error {
		return os.ErrNotExist
	}
	manager := BuildBaseManager("tsuru", "1.0", "", lookup)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	manager.e = &exiter
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"target"})
	expectedOutput := `.*: "target" is not a tsuru command. See "tsuru help".

Did you mean?
	target-add
	target-list
	target-remove
	target-set
`
	expectedOutput = strings.Replace(expectedOutput, "\n", "\\W", -1)
	expectedOutput = strings.Replace(expectedOutput, "\t", "\\W+", -1)
	c.Assert(stderr.String(), check.Matches, expectedOutput)
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestInvalidCommandFuzzyMatch02(c *check.C) {
	lookup := func(ctx *Context) error {
		return os.ErrNotExist
	}
	manager := BuildBaseManager("tsuru", "1.0", "", lookup)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	manager.e = &exiter
	manager.stdout = &stdout
	manager.stderr = &stderr
	manager.Run([]string{"target-lisr"})
	expectedOutput := `.*: "target-lisr" is not a tsuru command. See "tsuru help".

Did you mean?
	target-list
`
	expectedOutput = strings.Replace(expectedOutput, "\n", "\\W", -1)
	expectedOutput = strings.Replace(expectedOutput, "\t", "\\W+", -1)
	c.Assert(stderr.String(), check.Matches, expectedOutput)
	c.Assert(manager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestFileSystem(c *check.C) {
	fsystem = &fstest.RecordingFs{}
	c.Assert(filesystem(), check.DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), check.DeepEquals, fs.OsFs{})
}

func (s *S) TestValidateVersion(c *check.C) {
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
		c.Check(validateVersion(cs.support, cs.current), check.Equals, cs.expected)
	}
}

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
	return fmt.Errorf(c.msg)
}

type FailAndWorkCommand struct {
	calls int
}

func (c *FailAndWorkCommand) Info() *Info {
	return &Info{Name: "fail-and-work"}
}

func (c *FailAndWorkCommand) Run(context *Context, client *Client) error {
	c.calls++
	if c.calls == 1 {
		return errUnauthorized
	}
	fmt.Fprintln(context.Stdout, "worked nicely!")
	return nil
}

type SuccessLoginCommand struct{}

func (c *SuccessLoginCommand) Info() *Info {
	return &Info{Name: "login"}
}

func (c *SuccessLoginCommand) Run(context *Context, client *Client) error {
	fmt.Fprintln(context.Stdout, "logged in!")
	return nil
}

type UnauthorizedErrorCommand struct{}

func (c *UnauthorizedErrorCommand) Info() *Info {
	return &Info{Name: "unauthorized-error"}
}

func (c *UnauthorizedErrorCommand) Run(context *Context, client *Client) error {
	return errUnauthorized
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
	return &Info{
		Name:    "with-flags",
		Desc:    "with-flags doesn't do anything, really.",
		Usage:   "with-flags",
		MinArgs: c.minArgs,
	}
}

func (c *CommandWithFlags) Run(context *Context, client *Client) error {
	c.args = context.Args
	return nil
}

func (c *CommandWithFlags) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.IntVar(&c.age, "age", 0, "your age")
		c.fs.IntVar(&c.age, "a", 0, "your age")
	}
	return c.fs
}

type HelpCommandWithFlags struct {
	fs *gnuflag.FlagSet
	h  bool
}

func (c *HelpCommandWithFlags) Info() *Info {
	return &Info{
		Name:  "hflags",
		Desc:  "hflags doesn't do anything, really.",
		Usage: "hflags",
	}
}

func (c *HelpCommandWithFlags) Run(context *Context, client *Client) error {
	fmt.Fprintf(context.Stdout, "help called? %v", c.h)
	return nil
}

func (c *HelpCommandWithFlags) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.BoolVar(&c.h, "help", false, "help?")
		c.fs.BoolVar(&c.h, "h", false, "help?")
	}
	return c.fs
}
