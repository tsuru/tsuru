package main

import . "launchpad.net/gocheck"

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
