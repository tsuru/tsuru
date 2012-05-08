package gitosis

import (
	. "launchpad.net/gocheck"
	/* "github.com/timeredbull/tsuru/config" */
)

func (s *S) TestAddProject(c *C) {
	err := AddProject("someProject")
	c.Assert(err, IsNil)

	//should write to gitosis.conf
}
