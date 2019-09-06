// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/fs/fstest"
	check "gopkg.in/check.v1"
)

func (s *S) TestDeprecatedCommand(c *check.C) {
	var stdout, stderr bytes.Buffer
	cmd := TestCommand{}
	globalManager.RegisterDeprecated(&cmd, "bar")
	globalManager.stdout = &stdout
	globalManager.stderr = &stderr
	globalManager.Run([]string{"bar"})
	c.Assert(stdout.String(), check.Equals, "Running TestCommand")
	warnMessage := `WARNING: "bar" has been deprecated, please use "foo" instead.` + "\n\n"
	c.Assert(stderr.String(), check.Equals, warnMessage)
	stdout.Reset()
	stderr.Reset()
	globalManager.Run([]string{"foo"})
	c.Assert(stdout.String(), check.Equals, "Running TestCommand")
	c.Assert(stderr.String(), check.Equals, "")
}

func (s *S) TestDeprecatedCommandFlags(c *check.C) {
	var stdout, stderr bytes.Buffer
	cmd := CommandWithFlags{}
	globalManager.RegisterDeprecated(&cmd, "bar")
	globalManager.stdout = &stdout
	globalManager.stderr = &stderr
	globalManager.Run([]string{"bar", "--age", "10"})
	warnMessage := `WARNING: "bar" has been deprecated, please use "with-flags" instead.` + "\n\n"
	c.Assert(stderr.String(), check.Equals, warnMessage)
	c.Assert(cmd.age, check.Equals, 10)
}

func (s *S) TestRegister(c *check.C) {
	globalManager.Register(&TestCommand{})
	badCall := func() { globalManager.Register(&TestCommand{}) }
	c.Assert(badCall, check.PanicMatches, "command already registered: foo")
}

func (s *S) TestRegisterDeprecated(c *check.C) {
	originalCmd := &TestCommand{}
	globalManager.RegisterDeprecated(originalCmd, "bar")
	badCall := func() { globalManager.Register(originalCmd) }
	c.Assert(badCall, check.PanicMatches, "command already registered: foo")
	cmd, ok := globalManager.Commands["bar"].(*DeprecatedCommand)
	c.Assert(ok, check.Equals, true)
	c.Assert(cmd.Command, check.Equals, originalCmd)
	c.Assert(globalManager.Commands["foo"], check.Equals, originalCmd)
}

func (s *S) TestRegisterRemoved(c *check.C) {
	globalManager.RegisterRemoved("spoon", "There is no spoon.")
	_, ok := globalManager.Commands["spoon"].(*RemovedCommand)
	c.Assert(ok, check.Equals, true)
	var stdout, stderr bytes.Buffer
	globalManager.stdout = &stdout
	globalManager.stderr = &stderr
	globalManager.Run([]string{"spoon"})
	c.Assert(stdout.String(), check.Matches, "(?s).*This command was removed. There is no spoon.*")
}

func (s *S) TestRegisterTopic(c *check.C) {
	mngr := Manager{}
	mngr.RegisterTopic("target", "targeting everything!")
	c.Assert(mngr.topics["target"], check.Equals, "targeting everything!")
}

func (s *S) TestRegisterTopicDuplicated(c *check.C) {
	mngr := Manager{}
	mngr.RegisterTopic("target", "targeting everything!")
	defer func() {
		r := recover()
		c.Assert(r, check.NotNil)
	}()
	mngr.RegisterTopic("target", "wat")
}

func (s *S) TestRegisterTopicMultiple(c *check.C) {
	mngr := Manager{}
	mngr.RegisterTopic("target", "targeted")
	mngr.RegisterTopic("app", "what's an app?")
	expected := map[string]string{
		"target": "targeted",
		"app":    "what's an app?",
	}
	c.Assert(mngr.topics, check.DeepEquals, expected)
}

type TopicCommand struct {
	name     string
	executed bool
	args     []string
}

func (c *TopicCommand) Info() *Info {
	return &Info{
		Name:  c.name,
		Desc:  "desc " + c.name,
		Usage: "usage",
	}
}

func (c *TopicCommand) Run(context *Context, client *Client) error {
	c.executed = true
	c.args = context.Args
	return nil
}

func (s *S) TestImplicitTopicsHelp(c *check.C) {
	globalManager.Register(&TopicCommand{name: "foo-bar"})
	globalManager.Register(&TopicCommand{name: "foo-baz"})
	context := Context{[]string{"foo"}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
	command := help{manager: globalManager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	expected := `glb version 1.0.

The following commands are available in the "foo" topic:

  foo-bar              Desc foo-bar
  foo-baz              Desc foo-baz

Use glb help <commandname> to get more information about a command.
`
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestNormalizedCommandsExec(c *check.C) {
	cmds := map[string]*TopicCommand{
		"foo":             {name: "foo"},
		"foo-bar":         {name: "foo-bar"},
		"foo-bar-zzz":     {name: "foo-bar-zzz"},
		"foo-bar-zzz-a-b": {name: "foo-bar-zzz-a-b"},
	}
	for _, v := range cmds {
		globalManager.Register(v)
	}
	tests := []struct {
		args         []string
		expected     string
		expectedArgs []string
	}{
		{args: []string{"fo"}, expected: ""},
		{args: []string{"foo"}, expected: "foo"},
		{args: []string{"foo", "ba"}, expected: "foo", expectedArgs: []string{"ba"}},
		{args: []string{"foo-bar"}, expected: "foo-bar"},
		{args: []string{"foo-bar", "zz"}, expected: "foo-bar", expectedArgs: []string{"zz"}},
		{args: []string{"foo", "bar"}, expected: "foo-bar"},
		{args: []string{"foo", "bar", "zz"}, expected: "foo-bar", expectedArgs: []string{"zz"}},
		{args: []string{"foo-bar-zzz"}, expected: "foo-bar-zzz"},
		{args: []string{"foo-bar-zzz", "x"}, expected: "foo-bar-zzz", expectedArgs: []string{"x"}},
		{args: []string{"foo-bar", "zzz"}, expected: "foo-bar-zzz"},
		{args: []string{"foo", "bar-zzz"}, expected: "foo-bar-zzz"},
		{args: []string{"foo", "bar", "zzz"}, expected: "foo-bar-zzz"},
		{args: []string{"foo", "bar", "zzz", "x"}, expected: "foo-bar-zzz", expectedArgs: []string{"x"}},
		{args: []string{"foo-bar-zzz-a-b"}, expected: "foo-bar-zzz-a-b"},
		{args: []string{"foo-bar-zzz-a-b", "x"}, expected: "foo-bar-zzz-a-b", expectedArgs: []string{"x"}},
		{args: []string{"foo", "bar", "zzz", "a", "b"}, expected: "foo-bar-zzz-a-b"},
		{args: []string{"foo", "bar", "zzz", "a", "b", "x"}, expected: "foo-bar-zzz-a-b", expectedArgs: []string{"x"}},
	}
	for i, tt := range tests {
		globalManager.Run(tt.args)
		for k, v := range cmds {
			c.Assert(v.executed, check.Equals, k == tt.expected, check.Commentf("test %d, expected %s executed, got %s", i, tt.expected, k))
			if k == tt.expected {
				c.Assert(v.args, check.DeepEquals, tt.expectedArgs, check.Commentf("test %d", i))
			}
			v.executed = false
			v.args = nil
		}
	}
}

func (s *S) TestCustomLookup(c *check.C) {
	lookup := func(ctx *Context) error {
		fmt.Fprintf(ctx.Stdout, "test")
		return nil
	}
	var stdout, stderr bytes.Buffer
	mngr := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.Run([]string{"custom"})
	c.Assert(stdout.String(), check.Equals, "test")
}

func (s *S) TestCustomLookupNotFound(c *check.C) {
	lookup := func(ctx *Context) error {
		return ErrLookup
	}
	var stdout, stderr bytes.Buffer
	mngr := NewManager("glb", "0.x", "Foo-Tsuru", &stdout, &stderr, os.Stdin, lookup)
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.Register(&TestCommand{})
	mngr.Run([]string{"foo"})
	c.Assert(stdout.String(), check.Equals, "Running TestCommand")
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *check.C) {
	globalManager.Register(&ErrorCommand{msg: "You are wrong\n"})
	globalManager.Run([]string{"error"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldReturnStatus1WhenCommandFail(c *check.C) {
	globalManager.Register(&ErrorCommand{msg: "You are wrong\n"})
	globalManager.Run([]string{"error"})
	c.Assert(globalManager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestManagerRunShouldAppendNewLineOnErrorWhenItsNotPresent(c *check.C) {
	globalManager.Register(&ErrorCommand{msg: "You are wrong"})
	globalManager.Run([]string{"error"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: You are wrong\n")
}

func (s *S) TestManagerRunShouldNotWriteErrorOnStderrWhenErrAbortIsTriggered(c *check.C) {
	globalManager.Register(&ErrorCommand{msg: "abort"})
	globalManager.Run([]string{"error"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, "")
	c.Assert(globalManager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestManagerRunWithHTTPUnauthorizedError(c *check.C) {
	globalManager.Register(&UnauthorizedErrorCommand{})
	globalManager.Run([]string{"unauthorized-error"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, `Error: You're not authenticated or your session has expired. Please use "login" command for authentication.`+"\n")
}

func (s *S) TestManagerRunWithHTTPUnauthorizedErrorAndLoginRegistered(c *check.C) {
	expectedStderr := `Error: you're not authenticated or your session has expired.
Calling the "login" command...

`
	expectedStdout := `logged in!
worked nicely!
`
	globalManager.Register(&FailAndWorkCommand{})
	globalManager.Register(&SuccessLoginCommand{})
	globalManager.Run([]string{"fail-and-work"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, expectedStderr)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expectedStdout)
}

func (s *S) TestManagerRunWithGenericHTTPUnauthorizedErrorAndLoginRegistered(c *check.C) {
	expectedStderr := `Error: you're not authenticated or your session has expired.
Calling the "login" command...

`
	expectedStdout := `logged in!
worked nicely!
`

	globalManager.Register(&FailAndWorkCommandCustom{
		err: testStatusErr{status: http.StatusUnauthorized},
	})
	globalManager.Register(&SuccessLoginCommand{})
	globalManager.Run([]string{"fail-and-work"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, expectedStderr)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expectedStdout)
}

func (s *S) TestManagerRunWithGenericOtherError(c *check.C) {
	expectedStderr := `Error: my unauth
`
	expectedStdout := ``
	globalManager.Register(&FailAndWorkCommandCustom{
		err: testStatusErr{status: http.StatusBadRequest},
	})
	globalManager.Run([]string{"fail-and-work"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, expectedStderr)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expectedStdout)
}

func (s *S) TestManagerRunWithHTTPUnauthorizedErrorAndLoginFailure(c *check.C) {
	expected := `Error: you're not authenticated or your session has expired.
Calling the "login" command...
Error: You're not authenticated or your session has expired. Please use "login" command for authentication.
`
	globalManager.Register(&FailAndWorkCommand{})
	globalManager.Register(&UnauthorizedLoginErrorCommand{})
	globalManager.Run([]string{"fail-and-work"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestManagerRunLoginWithHTTPUnauthorizedError(c *check.C) {
	globalManager.Register(&UnauthorizedLoginErrorCommand{})
	globalManager.Run([]string{"login"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: unauthorized\n")
}

func (s *S) TestManagerRunWithErrorContainingBody(c *check.C) {
	globalManager.Register(&FailCommandCustom{
		err: errWithBody{},
	})
	globalManager.Run([]string{"failcmd"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, "Error: hey: my body\n")
}

func (s *S) TestManagerRunWithFlags(c *check.C) {
	cmd := &CommandWithFlags{}
	globalManager.Register(cmd)
	globalManager.Run([]string{"with-flags", "--age", "10"})
	c.Assert(cmd.fs.Parsed(), check.Equals, true)
	c.Assert(cmd.age, check.Equals, 10)
}

func (s *S) TestManagerRunWithFlagsAndArgs(c *check.C) {
	cmd := &CommandWithFlags{minArgs: 2}
	globalManager.Register(cmd)
	globalManager.Run([]string{"with-flags", "something", "--age", "20", "otherthing"})
	c.Assert(cmd.args, check.DeepEquals, []string{"something", "otherthing"})
}

func (s *S) TestManagerRunWithInvalidValueForFlag(c *check.C) {
	var exiter recordingExiter
	old := globalManager.e
	globalManager.e = &exiter
	defer func() {
		globalManager.e = old
	}()
	cmd := &CommandWithFlags{}
	globalManager.Register(cmd)
	globalManager.Run([]string{"with-flags", "--age", "tsuru"})
	c.Assert(cmd.fs.Parsed(), check.Equals, true)
	c.Assert(exiter.value(), check.Equals, 1)
}

func (s *S) TestRun(c *check.C) {
	globalManager.Register(&TestCommand{})
	globalManager.Run([]string{"foo"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *check.C) {
	globalManager.Run([]string{"bar"})
	c.Assert(globalManager.stderr.(*bytes.Buffer).String(), check.Equals, `glb: "bar" is not a glb command. See "glb help".`+"\n")
	c.Assert(globalManager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  version              Display the current version

Use glb help <commandname> to get more information about a command.
`
	globalManager.RegisterDeprecated(&login{}, "login")
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
	command := help{manager: globalManager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
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
  target               Something

Use glb help <topicname> to get more information about a topic.
`
	globalManager.Register(&login{})
	globalManager.RegisterTopic("target", "something")
	context := Context{[]string{}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
	command := help{manager: globalManager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpFromTopic(c *check.C) {
	expected := `glb version 1.0.

Targets

Tsuru likes to manage targets
`
	globalManager.RegisterTopic("target", "Targets\n\nTsuru likes to manage targets\n")
	context := Context{[]string{"target"}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
	command := help{manager: globalManager}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpCommandShouldBeRegisteredByDefault(c *check.C) {
	var stdout, stderr bytes.Buffer
	m := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	m.e = &exiter
	_, exists := m.Commands["help"]
	c.Assert(exists, check.Equals, true)
}

func (s *S) TestHelpReturnErrorIfTheGivenCommandDoesNotExist(c *check.C) {
	command := help{manager: globalManager}
	context := Context{[]string{"user-create"}, globalManager.stdout, globalManager.stderr, globalManager.stdin}
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
	globalManager.Run([]string{})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDashDashHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb command [args]

Available commands:
  help                 
  version              Display the current version

Use glb help <commandname> to get more information about a command.
`
	globalManager.Run([]string{"--help"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestRunCommandWithDashHelp(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	globalManager.Register(&TestCommand{})
	globalManager.Run([]string{"foo", "--help"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestRunCommandWithDashH(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	globalManager.Register(&TestCommand{})
	globalManager.Run([]string{"foo", "-h"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpShouldReturnHelpForACmd(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	globalManager.Register(&TestCommand{})
	globalManager.Run([]string{"help", "foo"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDashDashHelpShouldReturnHelpForACmd(c *check.C) {
	expected := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	globalManager.Register(&TestCommand{})
	globalManager.Run([]string{"--help", "foo"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDuplicateHelpFlag(c *check.C) {
	expected := "help called? true"
	globalManager.Register(&HelpCommandWithFlags{})
	globalManager.Run([]string{"hflags", "--help"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestDuplicateHFlag(c *check.C) {
	expected := "help called? true"
	globalManager.Register(&HelpCommandWithFlags{})
	globalManager.Run([]string{"hflags", "-h"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpFlaggedCommand(c *check.C) {
	expected := `glb version 1.0.

Usage: glb with-flags

with-flags doesn't do anything, really.

Flags:
  
  -a, --age  (= 0)
      your age
  
`
	globalManager.Register(&CommandWithFlags{})
	globalManager.Run([]string{"help", "with-flags"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpFlaggedMultilineCommand(c *check.C) {
	expected := `glb version 1.0.

Usage: glb with-flags

with-flags doesn't do anything, really.

Flags:
  
  -a, --age  (= 0)
      velvet darkness
      they fear
  
`
	globalManager.Register(&CommandWithFlags{multi: true})
	globalManager.Run([]string{"help", "with-flags"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
}

func (s *S) TestHelpDeprecatedCmd(c *check.C) {
	expectedStdout := `glb version 1.0.

Usage: glb foo

Foo do anything or nothing.

`
	expectedStderr := `WARNING: "bar" is deprecated. Showing help for "foo" instead.` + "\n\n"
	var stdout, stderr bytes.Buffer
	globalManager.stdout = &stdout
	globalManager.stderr = &stderr
	globalManager.RegisterDeprecated(&TestCommand{}, "bar")
	globalManager.Run([]string{"help", "bar"})
	c.Assert(stdout.String(), check.Equals, expectedStdout)
	c.Assert(stderr.String(), check.Equals, expectedStderr)
	stdout.Reset()
	stderr.Reset()
	globalManager.Run([]string{"help", "foo"})
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
	globalManager.stdout = &output
	globalManager.stderr = &output
	globalManager.RegisterDeprecated(&TestCommand{}, "bar")
	globalManager.Run([]string{"help", "bar"})
	c.Assert(output.String(), check.Equals, expected)
}

func (s *S) TestVersion(c *check.C) {
	var stdout, stderr bytes.Buffer
	mngr := NewManager("tsuru", "5.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	mngr.e = &exiter
	command := version{manager: mngr}
	context := Context{[]string{}, mngr.stdout, mngr.stderr, mngr.stdin}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(mngr.stdout.(*bytes.Buffer).String(), check.Equals, "tsuru version 5.0.\nAPI Server not found")
}

func (s *S) TestDashDashVersion(c *check.C) {
	expected := "glb version 1.0.\n"
	globalManager.Run([]string{"--version"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
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
	globalManager.Register(&ArgCmd{})
	globalManager.Run([]string{"arg"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(globalManager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestRunWithTooManyArguments(c *check.C) {
	expected := `glb version 1.0.

ERROR: wrong number of arguments.

Usage: glb arg [args]

some desc

Minimum # of arguments: 1
Maximum # of arguments: 2
`
	globalManager.Register(&ArgCmd{})
	globalManager.Run([]string{"arg", "param1", "param2", "param3"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, expected)
	c.Assert(globalManager.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestHelpShouldReturnUsageWithTheCommandName(c *check.C) {
	expected := `tsuru version 1.0.

Usage: tsuru foo

Foo do anything or nothing.

`
	var stdout, stderr bytes.Buffer
	mngr := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.Register(&TestCommand{})
	context := Context{[]string{"foo"}, mngr.stdout, mngr.stderr, mngr.stdin}
	command := help{manager: mngr}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(mngr.stdout.(*bytes.Buffer).String(), check.Equals, expected)
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
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	lgn, ok := mngr.Commands["login"]
	c.Assert(ok, check.Equals, true)
	c.Assert(lgn, check.FitsTypeOf, &login{})
}

func (s *S) TestLogoutIsRegistered(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	lgt, ok := mngr.Commands["logout"]
	c.Assert(ok, check.Equals, true)
	c.Assert(lgt, check.FitsTypeOf, &logout{})
}

func (s *S) TestTargetListIsRegistered(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	tgt, ok := mngr.Commands["target-list"]
	c.Assert(ok, check.Equals, true)
	c.Assert(tgt, check.FitsTypeOf, &targetList{})
}

func (s *S) TestTargetTopicIsRegistered(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	var exiter recordingExiter
	mngr.e = &exiter
	c.Assert(mngr.topics["target"], check.Equals, targetTopic)
}

func (s *S) TestVersionIsRegisteredByNewManager(c *check.C) {
	var stdout, stderr bytes.Buffer
	mngr := NewManager("tsuru", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	ver, ok := mngr.Commands["version"]
	c.Assert(ok, check.Equals, true)
	c.Assert(ver, check.FitsTypeOf, &version{})
}

func (s *S) TestUserInfoIsRegisteredByBaseManager(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	info, ok := mngr.Commands["user-info"]
	c.Assert(ok, check.Equals, true)
	c.Assert(info, check.FitsTypeOf, userInfo{})
}

func (s *S) TestInvalidCommandTopicMatch(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.stdout = &stdout
	mngr.stderr = &stderr
	mngr.Run([]string{"target"})
	expectedOutput := fmt.Sprintf(`%s

The following commands are available in the "target" topic:

  target-add           Adds a new entry to the list of available targets
  target-list          Displays the list of targets, marking the current
  target-remove        Remove a target from target-list (tsuru server)
  target-set           Change current target (tsuru server)

Use tsuru help <commandname> to get more information about a command.
`, targetTopic)
	c.Assert(stderr.String(), check.Equals, "")
	c.Assert(stdout.String(), check.Equals, expectedOutput)
	c.Assert(mngr.e.(*recordingExiter).value(), check.Equals, 0)
}

func (s *S) TestInvalidCommandFuzzyMatch02(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.stdout = &stdout
	mngr.stderr = &stderr
	mngr.Run([]string{"target-lisr"})
	expectedOutput := `.*: "target-lisr" is not a tsuru command. See "tsuru help".

Did you mean?
	target-list
`
	expectedOutput = strings.Replace(expectedOutput, "\n", "\\W", -1)
	expectedOutput = strings.Replace(expectedOutput, "\t", "\\W+", -1)
	c.Assert(stderr.String(), check.Matches, expectedOutput)
	c.Assert(mngr.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestInvalidCommandFuzzyMatch03(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.stdout = &stdout
	mngr.stderr = &stderr
	mngr.Run([]string{"list"})
	expectedOutput := `.*: "list" is not a tsuru command. See "tsuru help".

Did you mean?
	target-list
`
	expectedOutput = strings.Replace(expectedOutput, "\n", "\\W", -1)
	expectedOutput = strings.Replace(expectedOutput, "\t", "\\W+", -1)
	c.Assert(stderr.String(), check.Matches, expectedOutput)
	c.Assert(mngr.e.(*recordingExiter).value(), check.Equals, 1)
}

func (s *S) TestInvalidCommandFuzzyMatch04(c *check.C) {
	mngr := BuildBaseManager("tsuru", "1.0", "", nil)
	var stdout, stderr bytes.Buffer
	var exiter recordingExiter
	mngr.e = &exiter
	mngr.stdout = &stdout
	mngr.stderr = &stderr
	mngr.Run([]string{"not-command"})
	expectedOutput := `.*: "not-command" is not a tsuru command. See "tsuru help".
`
	expectedOutput = strings.Replace(expectedOutput, "\n", "\\W", -1)
	expectedOutput = strings.Replace(expectedOutput, "\t", "\\W+", -1)
	c.Assert(stderr.String(), check.Matches, expectedOutput)
	c.Assert(mngr.e.(*recordingExiter).value(), check.Equals, 1)
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
		{
			current:  "1.0-rc2",
			support:  "1.0-rc1",
			expected: true,
		},
		{
			current:  "1.0-rc1",
			support:  "1.0-rc1",
			expected: true,
		},
		{
			current:  "1.0-rc1",
			support:  "1.0-rc2",
			expected: false,
		},
		{
			current:  "1.0-rc1",
			support:  "1.0",
			expected: false,
		},
		{
			current:  "1.0",
			support:  "1.0-rc1",
			expected: true,
		},
	}
	for i, cs := range cases {
		c.Check(validateVersion(cs.support, cs.current), check.Equals, cs.expected, check.Commentf("error on %d", i))
	}
}

func (s *S) TestRunCancel(c *check.C) {
	cmd := &CancelableCommand{}
	cmd.running = make(chan struct{})
	cmd.canceled = make(chan struct{})
	go func() {
		<-cmd.running
		p, err := os.FindProcess(os.Getpid())
		c.Assert(err, check.IsNil)
		err = p.Signal(syscall.SIGINT)
		c.Assert(err, check.IsNil)
	}()
	globalManager.Register(cmd)
	globalManager.Run([]string{"foo"})
	c.Assert(globalManager.stdout.(*bytes.Buffer).String(), check.Equals, "Attempting command cancellation...\nCanceled.\n")
}

type recordingExiter int

func (e *recordingExiter) Exit(code int) {
	*e = recordingExiter(code)
}

func (e recordingExiter) value() int {
	return int(e)
}

var _ Cancelable = &CancelableCommand{}

type CancelableCommand struct {
	running  chan struct{}
	canceled chan struct{}
}

func (c *CancelableCommand) Info() *Info {
	return &Info{
		Name:  "foo",
		Desc:  "Foo do anything or nothing.",
		Usage: "foo",
	}
}

func (c *CancelableCommand) Run(context *Context, client *Client) error {
	c.running <- struct{}{}
	select {
	case <-c.canceled:
	case <-time.After(time.Second * 5):
		return fmt.Errorf("timeout waiting for cancellation")
	}
	return nil
}

func (c *CancelableCommand) Cancel(context Context, client *Client) error {
	fmt.Fprintln(context.Stdout, "Canceled.")
	c.canceled <- struct{}{}
	return nil
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
	multi   bool
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
		desc := "your age"
		if c.multi {
			desc = "velvet darkness\nthey fear"
		}
		c.fs.IntVar(&c.age, "age", 0, desc)
		c.fs.IntVar(&c.age, "a", 0, desc)
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

type FailAndWorkCommandCustom struct {
	calls int
	err   error
}

func (c *FailAndWorkCommandCustom) Info() *Info {
	return &Info{Name: "fail-and-work"}
}

func (c *FailAndWorkCommandCustom) Run(context *Context, client *Client) error {
	c.calls++
	if c.calls == 1 {
		return c.err
	}
	fmt.Fprintln(context.Stdout, "worked nicely!")
	return nil
}

type testStatusErr struct {
	status int
}

func (testStatusErr) Error() string {
	return "my unauth"
}

func (t testStatusErr) StatusCode() int {
	return t.status
}

type errWithBody struct{}

func (e errWithBody) Error() string {
	return "hey"
}

func (e errWithBody) Body() []byte {
	return []byte("my body")
}

type FailCommandCustom struct {
	err error
}

func (c *FailCommandCustom) Info() *Info {
	return &Info{Name: "failcmd"}
}

func (c *FailCommandCustom) Run(context *Context, client *Client) error {
	return c.err
}

func (s *S) TestVersionWithAPI(c *check.C) {
	var stdout, stderr bytes.Buffer
	mngr := NewManager("tsuru", "5.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	mngr.e = &exiter
	command := version{manager: mngr}
	context := Context{[]string{}, mngr.stdout, mngr.stderr, mngr.stdin}

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Send response to be tested
		rw.Write([]byte(`{"version":"1.7.4"}`))
	}))

	client := NewClient(&http.Client{}, &context, mngr)

	os.Setenv("TSURU_TARGET", ts.URL)
	defer os.Unsetenv("TSURU_TARGET")
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(mngr.stdout.(*bytes.Buffer).String(),
		check.Equals, "tsuru version 5.0.\nAPI Server version 1.7.4\n")
}
