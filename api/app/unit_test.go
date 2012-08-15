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

func (s *S) TestDestroyUnit(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(s.tmpdir)
	unit := Unit{Type: "django", Name: "myunit", Machine: 10, app: &App{JujuEnv: "zeta"}}
	out, err := unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "terminate-machine -e zeta 10")
}

func (s *S) TestUnitShouldBeARepositoryUnit(c *C) {
	var unit repository.Unit
	c.Assert(&Unit{}, Implements, &unit)
}

func (s *S) TestUnitShouldBeABinderUnit(c *C) {
	var unit bind.Unit
	c.Assert(&Unit{}, Implements, &unit)
}

func (s *S) TestStateMachineAgentPending(c *C) {
	u := Unit{MachineAgentState: "pending"}
	c.Assert(u.State(), Equals, "creating")
}

func (s *S) TestStateInstanceStatePending(c *C) {
	u := Unit{InstanceState: "pending"}
	c.Assert(u.State(), Equals, "creating")
}

func (s *S) TestStateInstanceStateError(c *C) {
	u := Unit{InstanceState: "error"}
	c.Assert(u.State(), Equals, "error")
}

func (s *S) TestStateAgentStatePending(c *C) {
	u := Unit{AgentState: "pending", InstanceState: ""}
	c.Assert(u.State(), Equals, "creating")
}

func (s *S) TestStateAgentAndInstanceRunning(c *C) {
	u := Unit{AgentState: "started", InstanceState: "running", MachineAgentState: "running"}
	c.Assert(u.State(), Equals, "started")
}

func (s *S) TestStateMachineAgentRunningAndInstanceAndAgentPending(c *C) {
	u := Unit{AgentState: "pending", InstanceState: "running", MachineAgentState: "running"}
	c.Assert(u.State(), Equals, "installing")
}

func (s *S) TestStateInstancePending(c *C) {
	u := Unit{AgentState: "not-started", InstanceState: "pending"}
	c.Assert(u.State(), Equals, "creating")
}

func (s *S) TestStateInstancePendingWhenMachineStateIsRunning(c *C) {
	u := Unit{AgentState: "not-started", MachineAgentState: "running"}
	c.Assert(u.State(), Equals, "creating")
}

func (s *S) TestStatePending(c *C) {
	u := Unit{MachineAgentState: "some-state", AgentState: "some-state", InstanceState: "some-other-state"}
	c.Assert(u.State(), Equals, "pending")
}
