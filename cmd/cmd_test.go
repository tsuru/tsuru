package cmd

import (
	"bytes"
	"errors"
	"io"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})
var manager Manager

func (s *S) SetUpTest(c *C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager(&stdout, &stderr)
}

type TestCommand struct {
	Name string
}

func (c *TestCommand) Info() *Info {
	return &Info{Name: c.Name}
}

func (c *TestCommand) Run(context *Context) error {
	io.WriteString(context.Stdout, "Running TestCommand")
	return nil
}

type ErrorCommand struct{}

func (c *ErrorCommand) Info() *Info {
	return &Info{Name: "error"}
}

func (c *ErrorCommand) Run(context *Context) error {
	return errors.New("You are wrong")
}

func (s *S) TestRegister(c *C) {
	manager.Register(&TestCommand{Name: "foo"})
	badCall := func() { manager.Register(&TestCommand{Name: "foo"}) }
	c.Assert(badCall, PanicMatches, "command already registered: foo")
}

func (s *S) TestManagerRunShouldWriteErrorsOnStderr(c *C) {
	manager.Register(&ErrorCommand{})
	manager.Run([]string{"error"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "You are wrong")
}

func (s *S) TestRun(c *C) {
	manager.Register(&TestCommand{Name: "foo"})
	manager.Run([]string{"foo"})
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, "Running TestCommand")
}

func (s *S) TestRunCommandThatDoesNotExist(c *C) {
	manager.Run([]string{"bar"})
	c.Assert(manager.Stderr.(*bytes.Buffer).String(), Equals, "command bar does not exist\n")
}
