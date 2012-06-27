package cmd

import (
	"bytes"
	"errors"
	"io"
	. "launchpad.net/gocheck"
	"os"
	"syscall"
)

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

type ErrorCommand struct{}

func (c *ErrorCommand) Info() *Info {
	return &Info{Name: "error"}
}

func (c *ErrorCommand) Run(context *Context, client Doer) error {
	return errors.New("You are wrong")
}

func (s *S) TestRegister(c *C) {
	manager.Register(&TestCommand{})
	badCall := func() { manager.Register(&TestCommand{}) }
	c.Assert(badCall, PanicMatches, "command already registered: foo")
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *C) {
	manager.Register(&ErrorCommand{})
	manager.Run([]string{"error"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "You are wrong")
}

func (s *S) TestRun(c *C) {
	manager.Register(&TestCommand{})
	manager.Run([]string{"foo"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "command bar does not exist\n")
}

type TicCmd struct {
	record *RecordCmd
}

func (c *TicCmd) Info() *Info {
	return &Info{Name: "tic", MinArgs: 1}
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

func (s *S) TestSubcommandWithArgs(c *C) {
	expected := []string{"arg1", "arg2"}
	cmd := &TicCmd{}
	manager.Register(cmd)
	manager.Run([]string{"tic", "record", "arg1", "arg2"})
	c.Assert(cmd.record.args, DeepEquals, expected)
}

func (s *S) TestHelp(c *C) {
	expected := `Usage: glb command [args]
`
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

func (s *S) TestRunWrongArgsNumberShouldRunsHelp(c *C) {
	expected := `Not enough arguments to call arg.

Usage: glb arg [args]

some desc

Minimum arguments: 1
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestRunWrongArgsNumberShouldRunsHelpForSubCmd(c *C) {
	expected := `Not enough arguments to call subargs.

Usage: glb arg subargs [args]

some subarg desc

Minimum arguments: 2
`
	manager.Register(&ArgCmd{})
	manager.Run([]string{"arg", "subargs"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestExtractCommandFromArgs(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{"arg", "ble"}), DeepEquals, []string{"arg"})
}

func (s *S) TestExtractCommandFromArgsForSubCmd(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{"arg", "subargs", "xpto", "ble"}), DeepEquals, []string{"arg", "subargs"})
}

func (s *S) TestExtractCommandFromArgsWithoutArgs(c *C) {
	manager.Register(&ArgCmd{})
	c.Assert(manager.extractCommandFromArgs([]string{}), DeepEquals, []string{})
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
