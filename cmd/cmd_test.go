package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})
var stdout, stderr bytes.Buffer
var manager Manager

func (s *S) SetUpTest(c *C) {
	manager = newManager(&stdout, &stderr)
}

var xpto = 0

type TestCommand struct {
	Name string
}

func (c *TestCommand) Info() *Info {
	return &Info{Name: c.Name}
}

func (c *TestCommand) Run() error {
	xpto = 1
	return nil
}

func (s *S) TestRegister(c *C) {
	manager.Register(&TestCommand{Name: "foo"})
	badCall := func() { manager.Register(&TestCommand{Name: "foo"}) }
	c.Assert(badCall, PanicMatches, "command already registered: foo")
}

func (s *S) TestNewManager(c *C) {
	c.Assert(manager.Stdout, Equals, &stdout)
	c.Assert(manager.Stderr, Equals, &stderr)
}

func (s *S) TestRun(c *C) {
	manager.Register(&TestCommand{Name: "foo"})
	manager.Run([]string{"foo"})
	c.Assert(xpto, Equals, 1)
}

func (s *S) TestRunCommandThatDoesNotExist(c *C) {
	manager.Run([]string{"bar"})
	c.Assert(stderr.String(), Equals, "command bar does not exist")
}
