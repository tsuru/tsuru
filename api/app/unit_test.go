// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/repository"
	. "launchpad.net/gocheck"
)

func (s *S) TestCommand(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(s.tmpdir)
	u := Unit{
		Type:              "django",
		Name:              "myUnit",
		Machine:           1,
		app:               &App{},
		InstanceState:     "running",
		AgentState:        "started",
		MachineAgentState: "running",
	}
	output, err := u.Command(nil, nil, "uname")
	c.Assert(err, IsNil)
	c.Assert(string(output), Matches, `.* \d uname`)
}

func (s *S) TestCommandShouldAcceptMultipleParams(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Type:              "django",
		Name:              "myUnit",
		Machine:           1,
		app:               &App{},
		InstanceState:     "running",
		AgentState:        "started",
		MachineAgentState: "running",
	}
	out, err := u.Command(nil, nil, "uname", "-a")
	c.Assert(string(out), Matches, `.* \d uname -a`)
}

func (s *S) TestCommandWithCustomStdout(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Type:              "django",
		Name:              "myUnit",
		Machine:           1,
		app:               &App{},
		InstanceState:     "running",
		AgentState:        "started",
		MachineAgentState: "running",
	}
	var b bytes.Buffer
	u.Command(&b, nil, "uname", "-a")
	c.Assert(b.String(), Matches, `.* \d uname -a`)
}

func (s *S) TestCommandWithCustomStderr(c *C) {
	dir, err := commandmocker.Error("juju", "$*", 42)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	u := Unit{
		Type:              "django",
		Name:              "myUnit",
		Machine:           1,
		app:               &App{},
		InstanceState:     "running",
		AgentState:        "started",
		MachineAgentState: "running",
	}
	var b bytes.Buffer
	_, err = u.Command(nil, &b, "uname", "-a")
	c.Assert(err, NotNil)
	c.Assert(b.String(), Matches, `.* \d uname -a`)
}

func (s *S) TestCommandReturnErrorIfTheUnitIsNotStarted(c *C) {
	u := Unit{
		Type:              "django",
		Name:              "myUnit",
		Machine:           1,
		app:               &App{},
		InstanceState:     "running",
		AgentState:        "not-started",
		MachineAgentState: "running",
	}
	_, err := u.Command(nil, nil, "uname", "-a")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Unit must be started to run commands, but it is "+u.State()+".")
}

func (s *S) TestExecuteHook(c *C) {
	appUnit := Unit{Type: "django", Name: "myUnit", app: &App{}, MachineAgentState: "running", AgentState: "started", InstanceState: "running"}
	_, err := appUnit.executeHook(nil, nil, "requirements")
	c.Assert(err, IsNil)
}

func (s *S) TestExecuteHookWithCustomStdout(c *C) {
	dir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	appUnit := Unit{Type: "django", Name: "myUnit", app: &App{}, MachineAgentState: "running", AgentState: "started", InstanceState: "running"}
	var b bytes.Buffer
	_, err = appUnit.executeHook(&b, nil, "requirements")
	c.Assert(err, IsNil)
	c.Assert(b.String(), Matches, `.* \d /var/lib/tsuru/hooks/requirements`)
}

func (s *S) TestExecuteHookWithCustomStderr(c *C) {
	dir, err := commandmocker.Error("juju", "$*", 42)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	appUnit := Unit{Type: "django", Name: "myUnit", app: &App{}, MachineAgentState: "running", AgentState: "started", InstanceState: "running"}
	var b bytes.Buffer
	_, err = appUnit.executeHook(nil, &b, "requirements")
	c.Assert(err, NotNil)
	c.Assert(b.String(), Matches, `.* \d /var/lib/tsuru/hooks/requirements`)
}

func (s *S) TestDestroyUnit(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(s.tmpdir)
	unit := Unit{Type: "django", Name: "myunit", Machine: 10, app: &App{}}
	out, err := unit.destroy()
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, "terminate-machine 10")
}

func (s *S) TestCantDestroyAUnitWithMachine0(c *C) {
	u := Unit{Type: "django", Name: "nova-era", Machine: 0, app: &App{}}
	out, err := u.destroy()
	c.Assert(out, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^No machine associated.$")
}

func (s *S) TestGetName(c *C) {
	u := Unit{app: &App{Name: "2112"}}
	c.Assert(u.GetName(), Equals, "2112")
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

func (s *S) TestStateAgentStateDown(c *C) {
	u := Unit{InstanceState: "running", MachineAgentState: "running", AgentState: "down"}
	c.Assert(u.State(), Equals, "down")
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

func (s *S) TestStateMachineAgentNotStarted(c *C) {
	u := Unit{AgentState: "pending", InstanceState: "running", MachineAgentState: "not-started"}
	c.Assert(u.State(), Equals, "creating")
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

func (s *S) TestStateError(c *C) {
	u := Unit{MachineAgentState: "start-error", AgentState: "pending", InstanceState: "running"}
	c.Assert(u.State(), Equals, "error")
}
