package errors

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestErrorMethodShouldReturnTheMessageString(c *C) {
	e := Http{500, "Internal server error"}
	c.Assert(e.Error(), Equals, e.Message)
}
