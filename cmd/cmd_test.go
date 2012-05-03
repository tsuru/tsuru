package cmd

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

type TestCommand struct {
	Name string
}

func (c *TestCommand) Info() *Info {
	return &Info{Name: c.Name}
}

func (c *TestCommand) Run() error {
	return nil
}

func (s *S) TestRegister(c *C) {
	manager := &Manager{}
	manager.Register(&TestCommand{Name: "foo"})
	badCall := func() { manager.Register(&TestCommand{Name: "foo"}) }
	c.Assert(badCall, PanicMatches, "command already registered: foo")
}
