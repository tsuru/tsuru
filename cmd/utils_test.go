package main

import (
	. "launchpad.net/gocheck"
	"os"
)

func (s *S) TestWriteToken(c *C) {
	err := WriteToken("abc")
	c.Assert(err, IsNil)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "abc")
}

func (s *S) TestReadToken(c *C) {
	err := WriteToken("123")
	c.Assert(err, IsNil)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "123")
}

func (s *S) TestReadTokenNotReturnErrorWhenTokenDoesNotExists(c *C) {
	err := os.Remove(os.ExpandEnv("${HOME}/.tsuru_token"))
	c.Assert(err, IsNil)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "")
}
