package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	. "launchpad.net/gocheck"
)

func (s *S) TestSetEnvMessage(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "the-wicker-man", Teams: []auth.Team{s.team}}
	msg := Message{
		app:     &app,
		env:     map[string]string{"PATH": "/"},
		kind:    "set",
		success: make(chan bool),
	}
	env <- msg
	c.Assert(<-msg.success, Equals, true)
	c.Assert(commandmocker.Ran(dir), Equals, true)
}

func (s *S) TestUnsetEnvMessage(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "time", Teams: []auth.Team{s.team}}
	msg := Message{
		app:     &app,
		env:     map[string]string{"DATABASE_HOST": ""},
		kind:    "unset",
		success: make(chan bool),
	}
	env <- msg
	c.Assert(<-msg.success, Equals, true)
	c.Assert(commandmocker.Ran(dir), Equals, true)
}

func (s *S) TestDoesNotSendInTheSuccessChannelIfItIsNil(c *C) {
	defer func() {
		r := recover()
		c.Assert(r, IsNil)
	}()
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "rainmaker", Teams: []auth.Team{s.team}}
	err = app.Create()
	c.Assert(err, IsNil)
	msg := Message{
		app:  &app,
		env:  map[string]string{"PATH": "/"},
		kind: "set",
	}
	env <- msg
}

func (s *S) TestFilterLines(c *C) {
	lines := []byte("line1\nline2\n3line")
	expected := []byte("line1\nline2")
	got := excludeLines(lines, `^\dline`)
	c.Assert(string(got), Equals, string(expected))
}

func (s *S) TestFilterLinesPanicsIfTheRegexIsNotValid(c *C) {
	defer func() {
		r := recover()
		c.Assert(r, NotNil)
	}()
	excludeLines(nil, "^[asa")
}
