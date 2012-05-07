package cmd

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestString(c *C) {
	table := NewTable()
	c.Assert(table.String(), Equals, "")
}
