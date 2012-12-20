// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"errors"
	"github.com/globocom/tsuru/provision"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestFindApp(c *C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	p.apps = []provision.App{app}
	c.Assert(p.FindApp(app), Equals, 0)
	otherapp := *app
	otherapp.name = "blue-sector"
	c.Assert(p.FindApp(&otherapp), Equals, -1)
}

func (s *S) TestGetCmds(c *C) {
	app := NewFakeApp("enemy-within", "rush", 1)
	p := NewFakeProvisioner()
	p.cmds = []Cmd{
		{Cmd: "ls -lh", App: app},
		{Cmd: "ls -lah", App: app},
	}
	c.Assert(p.GetCmds("ls -lh", app), HasLen, 1)
	c.Assert(p.GetCmds("l", app), HasLen, 0)
	c.Assert(p.GetCmds("", app), HasLen, 2)
	otherapp := *app
	otherapp.name = "enemy-without"
	c.Assert(p.GetCmds("ls -lh", &otherapp), HasLen, 0)
	c.Assert(p.GetCmds("", &otherapp), HasLen, 0)
}

func (s *S) TestGetUnits(c *C) {
	list := []provision.Unit{
		{"chain-lighting/0", "chain-lighting", "django", 1, "10.10.10.10", provision.StatusStarted},
		{"chain-lighting/1", "chain-lighting", "django", 2, "10.10.10.15", provision.StatusStarted},
	}
	app := NewFakeApp("chain-lighting", "rush", 1)
	p := NewFakeProvisioner()
	p.units["chain-lighting"] = list
	units := p.GetUnits(app)
	c.Assert(units, DeepEquals, list)
}

func (s *S) TestPrepareOutput(c *C) {
	output := []byte("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	got := <-p.outputs
	c.Assert(string(got), Equals, string(output))
}

func (s *S) TestPrepareFailure(c *C) {
	err := errors.New("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareFailure("Rush", err)
	got := <-p.failures
	c.Assert(got.method, Equals, "Rush")
	c.Assert(got.err.Error(), Equals, "the body eletric")
}

func (s *S) TestProvision(c *C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, IsNil)
	c.Assert(p.apps, DeepEquals, []provision.App{app})
}

func (s *S) TestProvisionWithPreparedFailure(c *C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Provision", errors.New("Failed to provision."))
	err := p.Provision(app)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to provision.")
}

func (s *S) TestDoubleProvision(c *C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, IsNil)
	err = p.Provision(app)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "App already provisioned.")
}

func (s *S) TestDestroy(c *C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.apps = []provision.App{app}
	err := p.Destroy(app)
	c.Assert(err, IsNil)
	c.Assert(p.FindApp(app), Equals, -1)
	c.Assert(p.apps, DeepEquals, []provision.App{})
}

func (s *S) TestDestroyWithPreparedFailure(c *C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Destroy", errors.New("Failed to destroy."))
	err := p.Destroy(app)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to destroy.")
}

func (s *S) TestDestroyNotProvisionedApp(c *C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Destroy(app)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "App is not provisioned.")
}

func (s *S) TestAddUnits(c *C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, IsNil)
	c.Assert(p.units["mystic-rhythms"], HasLen, 2)
	c.Assert(units, HasLen, 2)
}

func (s *S) TestAddZeroUnits(c *C) {
	p := NewFakeProvisioner()
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Cannot add 0 units.")
}

func (s *S) TestAddUnitsUnprovisionedApp(c *C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	units, err := p.AddUnits(app, 1)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "App is not provisioned.")
}

func (s *S) TestAddUnitsFailure(c *C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("AddUnits", errors.New("Cannot add more units."))
	units, err := p.AddUnits(nil, 10)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Cannot add more units.")
}

func (s *S) TestExecuteCommand(c *C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 0)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	err := p.ExecuteCommand(&buf, nil, app, "ls", "-l")
	c.Assert(err, IsNil)
	cmds := p.GetCmds("ls", app)
	c.Assert(cmds, HasLen, 1)
	c.Assert(buf.String(), Equals, string(output))
}

func (s *S) TestExecuteCommandFailureNoOutput(c *C) {
	app := NewFakeApp("manhattan-project", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	err := p.ExecuteCommand(nil, nil, app, "ls", "-l")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to run command.")
}

func (s *S) TestExecuteCommandWithOutputAndFailure(c *C) {
	var buf bytes.Buffer
	app := NewFakeApp("marathon", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	p.PrepareOutput([]byte("myoutput!"))
	err := p.ExecuteCommand(nil, &buf, app, "ls", "-l")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to run command.")
	c.Assert(buf.String(), Equals, "myoutput!")
}

func (s *S) TestExecuteComandTimeout(c *C) {
	app := NewFakeApp("territories", "rush", 1)
	p := NewFakeProvisioner()
	err := p.ExecuteCommand(nil, nil, app, "ls -l")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "FakeProvisioner timed out waiting for output.")
}

func (s *S) TestCollectStatus(c *C) {
	p := NewFakeProvisioner()
	p.apps = []provision.App{
		NewFakeApp("red-lenses", "rush", 1),
		NewFakeApp("between-the-wheels", "rush", 1),
		NewFakeApp("the-big-money", "rush", 1),
		NewFakeApp("grand-designs", "rush", 1),
	}
	expected := []provision.Unit{
		{"red-lenses/0", "red-lenses", "rush", 1, "10.10.10.1", "started"},
		{"between-the-wheels/0", "between-the-wheels", "rush", 2, "10.10.10.2", "started"},
		{"the-big-money/0", "the-big-money", "rush", 3, "10.10.10.3", "started"},
		{"grand-designs/0", "grand-designs", "rush", 4, "10.10.10.4", "started"},
	}
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, expected)
}

func (s *S) TestCollectStatusPreparedFailure(c *C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("CollectStatus", errors.New("Failed to collect status."))
	units, err := p.CollectStatus()
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Failed to collect status.")
}

func (s *S) TestCollectStatusNoApps(c *C) {
	p := NewFakeProvisioner()
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
}
