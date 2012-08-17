package cmd

import (
	"bytes"
	"errors"
	"github.com/timeredbull/tsuru/fs"
	"github.com/timeredbull/tsuru/fs/testing"
	"io"
	. "launchpad.net/gocheck"
	"os"
	"strings"
	"syscall"
)

type RecordingExiter int

func (e *RecordingExiter) Exit(code int) {
	*e = RecordingExiter(code)
}

func (e RecordingExiter) value() int {
	return int(e)
}

func (s *S) patchStdin(c *C, content []byte) {
	f, err := os.OpenFile("/tmp/passwdfile.txt", syscall.O_RDWR|syscall.O_NDELAY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	c.Assert(err, IsNil)
	n, err := f.Write(content)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(content))
	ret, err := f.Seek(0, 0)
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, int64(0))
	s.stdin = os.Stdin
	os.Stdin = f
}

func (s *S) unpatchStdin() {
	os.Stdin = s.stdin
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

func (c *TestCommand) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"ble": &TestSubCommand{},
	}
}

type TestSubCommand struct{}

func (c *TestSubCommand) Info() *Info {
	return &Info{
		Name:  "ble",
		Desc:  "Ble do anything or nothing.",
		Usage: "foo ble",
	}
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
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "You are wrong\n")
}

func (s *S) TestManagerRunShouldReturnStatus1WhenCommandFail(c *C) {
	manager.Register(&ErrorCommand{msg: "You are wrong\n"})
	manager.Run([]string{"error"})
	c.Assert(manager.e.(*RecordingExiter).value(), Equals, 1)
}

func (s *S) TestManagerRunShouldAppendNewLineOnErrorWhenItsNotPresent(c *C) {
	manager.Register(&ErrorCommand{msg: "You are wrong"})
	manager.Run([]string{"error"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "You are wrong\n")
}

func (s *S) TestRun(c *C) {
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "command bar does not exist\n")
	c.Assert(manager.e.(*RecordingExiter).value(), Equals, 1)
}

type TicCmd struct {
	record *RecordCmd
}

func (c *TicCmd) Info() *Info {
	return &Info{
		Name:    "tic",
		MinArgs: 1,
		Usage:   "tic tac|record",
		Desc:    "some tic command",
	}
}

func (c *TicCmd) Subcommands() map[string]interface{} {
	c.record = &RecordCmd{}
	return map[string]interface{}{"tac": &TacCmd{}, "record": c.record}
}

type TacCmd struct{}

func (c *TacCmd) Info() *Info {
	return &Info{Name: "tac"}
}

func (c *TacCmd) Run(context *Context, client Doer) error {
	io.WriteString(context.Stdout, "Running tac subcommand")
	return nil
}

type RecordCmd struct {
	args []string
}

func (c *RecordCmd) Info() *Info {
	return &Info{Name: "record", MinArgs: 2}
}

func (c *RecordCmd) Run(context *Context, client Doer) error {
	c.args = context.Args
	return nil
}

func (s *S) TestSubcommand(c *C) {
	manager.Register(&TicCmd{})
	manager.Run([]string{"tic", "tac"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Running tac subcommand")
}

func (s *S) TestErrorWhenSubcommandDoesntExists(c *C) {
	manager.Register(&TicCmd{})
	manager.Run([]string{"tic", "toe"})
	obtained := strings.Replace(manager.Stdout.(*bytes.Buffer).String(), "\n", " ", -1)
	c.Assert(obtained, Matches, ".*subcommand toe does not exist.*")
	c.Assert(obtained, Matches, ".*tic tac|record.*")
	c.Assert(obtained, Matches, ".*some tic command.*")
}

func (s *S) TestSubcommandWithArgs(c *C) {
	expected := []string{"arg1", "arg2"}
	cmd := &TicCmd{}
	manager.Register(cmd)
	manager.Run([]string{"tic", "record", "arg1", "arg2"})
	c.Assert(cmd.record.args, DeepEquals, expected)
}

func (s *S) TestHelp(c *C) {
	expected := `Usage: glb command [args]

Available commands:
  help
  user-create

Run glb help <commandname> to get more information about a specific command.
`
	manager.Register(&UserCreate{})
	context := Context{[]string{}, []string{}, manager.Stdout, manager.Stderr}
	command := Help{manager: &manager}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestHelpCommandShouldBeRegisteredByDefault(c *C) {
	var stdout, stderr bytes.Buffer
	m := NewManager("tsuru", &stdout, &stderr)
	_, exists := m.Commands["help"]
	c.Assert(exists, Equals, true)
}

func (s *S) TestRunWithoutArgsShouldRunsHelp(c *C) {
	expected := `Usage: glb command [args]

Available commands:
  help

Run glb help <commandname> to get more information about a specific command.
`
	manager.Run([]string{})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestHelpShouldReturnsHelpForACmd(c *C) {
	expected := `Usage: glb foo

Foo do anything or nothing.
`
	manager.Register(&TestCommand{})
	manager.Run([]string{"help", "foo"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestHelpShouldReturnsHelpForASubCmd(c *C) {
	expected := `Usage: glb foo ble

Ble do anything or nothing.
`
	manager.Register(&TestCommand{})
	manager.Run([]string{"help", "foo", "ble"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
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

func (c *ArgCmd) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"subargs": &ArgSubCmd{},
	}
}

type ArgSubCmd struct{}

func (c *ArgSubCmd) Info() *Info {
	return &Info{
		Name:    "subargs",
		MinArgs: 2,
		Usage:   "arg subargs [args]",
		Desc:    "some subarg desc",
	}
}

func (cmd *ArgSubCmd) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"subsubcmd": &ArgSubSubcmd{},
	}
}

type ArgSubSubcmd struct{}

func (cmd *ArgSubSubcmd) Info() *Info {
	return &Info{
		Name:  "subsubcmd",
		Desc:  "just some sub sub cmd",
		Usage: "tsuru arg subargs subsubcmd <arg>",
	}
}

func (cmd *ArgSubSubcmd) Run(ctx *Context, client Doer) error {
	return nil
}

func (s *S) TestRunWrongArgsNumberShouldRunsHelpAndReturnStatus1(c *C) {
	expected := `Not enough arguments to call arg.

Usage: glb arg [args]

some desc

Minimum arguments: 1
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	c.Assert(manager.e.(*RecordingExiter).value(), Equals, 1)
}

func (s *S) TestRunWrongArgsNumberShouldRunsHelpForSubCmdAndReturnsStatus1(c *C) {
	expected := `Not enough arguments to call subargs.

Usage: glb arg subargs [args]

some subarg desc

Minimum arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg", "subargs"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
	c.Assert(manager.e.(*RecordingExiter).value(), Equals, 1)
}

func (s *S) TestExtractCommandFromArgs(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{"arg", "ble"}), DeepEquals, []string{"arg"})
}

func (s *S) TestExtractCommandFromArgsWithThreeSubcmds(c *C) {
	manager := BuildBaseManager("tsuru")
	manager.Register(&ArgCmd{})
	args := manager.extractCommandFromArgs([]string{"arg", "subargs", "subsubcmd", "args"})
	c.Assert(args, DeepEquals, []string{"arg", "subargs", "subsubcmd"})
}

func (s *S) TestExtractCommandFromArgsForSubCmd(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{"arg", "subargs", "xpto", "ble"}), DeepEquals, []string{"arg", "subargs"})
}

func (s *S) TestExtractCommandFromArgsWithoutArgs(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{}), DeepEquals, []string{})
}

func (s *S) TestGetSubcommand(c *C) {
	manager.Register(&ArgCmd{})
	var cmd interface{}
	cmd = &ArgCmd{}
	_, ok := cmd.(CommandContainer)
	c.Assert(ok, Equals, true)
	cmds := []string{"arg", "subargs", "subsubcmd", "argument"}
	got := getSubcommand(cmd, cmds)
	c.Assert(got, FitsTypeOf, &ArgSubSubcmd{})
}

func (s *S) TestHelpShouldReturnUsageWithTheCommandName(c *C) {
	expected := `Usage: tsuru foo ble

Ble do anything or nothing.
`
	var stdout, stderr bytes.Buffer
	manager := NewManager("tsuru", &stdout, &stderr)
	manager.Register(&TestCommand{})
	context := Context{[]string{}, []string{"foo", "ble"}, manager.Stdout, manager.Stderr}
	command := Help{manager: &manager}
	err := command.Run(&context, nil)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
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
	c.Assert(m.finisher(), FitsTypeOf, OsExiter{})
}

func (s *S) TestFinisherReturnTheDefinedE(c *C) {
	var exiter RecordingExiter
	m := Manager{e: &exiter}
	c.Assert(m.finisher(), FitsTypeOf, &exiter)
}

func (s *S) TestLoginIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	login, ok := manager.Commands["login"]
	c.Assert(ok, Equals, true)
	c.Assert(login, FitsTypeOf, &Login{})
}

func (s *S) TestLogoutIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	logout, ok := manager.Commands["logout"]
	c.Assert(ok, Equals, true)
	c.Assert(logout, FitsTypeOf, &Logout{})
}

func (s *S) TestUserCreateIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	user, ok := manager.Commands["user-create"]
	c.Assert(ok, Equals, true)
	c.Assert(user, FitsTypeOf, &UserCreate{})
}

func (s *S) TestTeamCreateIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	create, ok := manager.Commands["team-create"]
	c.Assert(ok, Equals, true)
	c.Assert(create, FitsTypeOf, &TeamCreate{})
}

func (s *S) TestTeamListIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	list, ok := manager.Commands["team-list"]
	c.Assert(ok, Equals, true)
	c.Assert(list, FitsTypeOf, &TeamList{})
}

func (s *S) TestTeamAddUserIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	adduser, ok := manager.Commands["team-user-add"]
	c.Assert(ok, Equals, true)
	c.Assert(adduser, FitsTypeOf, &TeamUserAdd{})
}

func (s *S) TestTeamRemoveUserIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	removeuser, ok := manager.Commands["team-user-remove"]
	c.Assert(ok, Equals, true)
	c.Assert(removeuser, FitsTypeOf, &TeamUserRemove{})
}

func (s *S) TestTargetIsRegistered(c *C) {
	manager := BuildBaseManager("tsuru")
	target, ok := manager.Commands["target"]
	c.Assert(ok, Equals, true)
	c.Assert(target, FitsTypeOf, &Target{})
}

func (s *S) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
}
