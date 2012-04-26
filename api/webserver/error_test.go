package webserver

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestErrorFunctionShouldReturnAnHttpErrorInstance(c *C) {
	e := Error(500, "Internal server error")
	c.Assert(e, FitsTypeOf, &HttpError{})
}

func (s *S) TestErrorMethodShouldReturnTheMessageString(c *C) {
	e := Error(500, "Internal server error")
	c.Assert(e.Error(), Equals, e.message)
}
