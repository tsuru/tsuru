package unit

import (
	"github.com/timeredbull/commandmocker"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommand(c *C) {
	u := Unit{Type: "django", Name: "myUnit", Machine: 1}
	output, err := u.Command("uname")
	c.Assert(err, IsNil)
	c.Assert(string(output), Equals, "Linux")
}

func (s *S) TestCommandShouldAcceptMultipleParams(c *C) {
	output := "$*"
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{Type: "django", Name: "myUnit", Machine: 1}
	out, err := u.Command("uname", "-a")
	c.Assert(string(out), Matches, ".* uname -a")
}

func (s *S) TestExecuteHook(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit"}
	_, err := appUnit.ExecuteHook("requirements")
	c.Assert(err, IsNil)
}
