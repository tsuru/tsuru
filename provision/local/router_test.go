package local

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestAddRoute(c *C) {
	err := AddRoute("name", "127.0.0.1")
	c.Assert(err, IsNil)
}
