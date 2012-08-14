package app

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/repository"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommand(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(s.tmpdir)
	u := Unit{Type: "django", Name: "myUnit", Machine: 1, app: &App{JujuEnv: "alpha"}}
	output, err := u.Command("uname")
	c.Assert(err, IsNil)
	c.Assert(string(output), Matches, `.* -e alpha \d uname`)
}

func (s *S) TestCommandShouldAcceptMultipleParams(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{Type: "django", Name: "myUnit", Machine: 1, app: &App{JujuEnv: "alpha"}}
	out, err := u.Command("uname", "-a")
	c.Assert(string(out), Matches, `.* -e alpha \d uname -a`)
}

func (s *S) TestExecuteHook(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit", app: &App{JujuEnv: "beta"}}
	_, err := appUnit.ExecuteHook("requirements")
	c.Assert(err, IsNil)
}

func (s *S) TestUnitShouldBeARepositoryUnit(c *C) {
	var unit repository.Unit
	c.Assert(&Unit{}, Implements, &unit)
}

func (s *S) TestUnitShouldBeABinderUnit(c *C) {
	var unit bind.Unit
	c.Assert(&Unit{}, Implements, &unit)
}
