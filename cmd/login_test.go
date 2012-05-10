package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestLoginRun(c *C) {
	expected := "Successfully logged!"
	context := Context{[]string{"foo@foo.com", "bar123"}, manager.Stdout, manager.Stderr}
	client := NewClient(&http.Client{Transport: &transport{msg: `{"token": "sometoken"}`, status: http.StatusOK}})
	command := LoginCommand{}
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)

	token, err := ReadToken()
	c.Assert(token, Equals, "sometoken")
}
