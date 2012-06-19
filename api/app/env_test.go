package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	. "launchpad.net/gocheck"
)

func (s *S) TestRewriteEnvMessage(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "time", Teams: []auth.Team{s.team}}
	msg := Message{
		app:     &app,
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
		app: &app,
	}
	env <- msg
}
