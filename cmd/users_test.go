package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAddCommandRun(c *C) {
	expected := `Creating new user: foo@foo.com
OK`
	context := Context{[]string{"foo@foo.com", "bar123"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusCreated}})
	command := AddUserCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}
