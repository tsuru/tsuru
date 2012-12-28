// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"errors"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	. "launchpad.net/gocheck"
	"reflect"
	"time"
)

func (s *S) TestShouldBeRegistered(c *C) {
	p, err := provision.Get("juju")
	c.Assert(err, IsNil)
	c.Assert(p, FitsTypeOf, &JujuProvisioner{})
}

func (s *S) TestELBSupport(c *C) {
	config.Set("juju:use-elb", true)
	p := JujuProvisioner{}
	c.Assert(p.elbSupport(), Equals, true)
	config.Set("juju:use-elb", false)
	c.Assert(p.elbSupport(), Equals, true) // Read config only once.
	p = JujuProvisioner{}
	c.Assert(p.elbSupport(), Equals, false)
}

func (s *S) TestProvision(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	err = p.Provision(app)
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, "deploy --repository /home/charms local:python trace")
}

func (s *S) TestProvisionFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	err = p.Provision(app)
	c.Assert(err, NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(pErr.Reason, Equals, "juju failed")
	c.Assert(pErr.Err.Error(), Equals, "exit status 1")
}

func (s *S) TestDestroy(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("cribcaged", "python", 3)
	p := JujuProvisioner{}
	err = p.Destroy(app)
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := []string{
		"destroy-service", "cribcaged",
		"terminate-machine", "1",
		"terminate-machine", "2",
		"terminate-machine", "3",
	}
	ran := make(chan bool, 1)
	go func() {
		for {
			if reflect.DeepEqual(commandmocker.Parameters(tmpdir), expected) {
				ran <- true
			}
		}
	}()
	select {
	case <-ran:
	case <-time.After(2e9):
		c.Errorf("Did not run terminate-machine commands after 2 seconds.")
	}
	c.Assert(commandmocker.Parameters(tmpdir), DeepEquals, expected)
}

func (s *S) TestDestroyFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed to destroy the machine", 25)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("idioglossia", "static", 1)
	p := JujuProvisioner{}
	err = p.Destroy(app)
	c.Assert(err, NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(pErr.Reason, Equals, "juju failed to destroy the machine")
	c.Assert(pErr.Err.Error(), Equals, "exit status 25")
}

func (s *S) TestAddUnits(c *C) {
	tmpdir, err := commandmocker.Add("juju", addUnitsOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("resist", "rush", 0)
	p := JujuProvisioner{}
	units, err := p.AddUnits(app, 4)
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 4)
	names := make([]string, len(units))
	for i, unit := range units {
		names[i] = unit.Name
	}
	expected := []string{"resist/3", "resist/4", "resist/5", "resist/6"}
	c.Assert(names, DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expectedParams := []string{
		"set", "resist", "app-repo=" + repository.GetReadOnlyUrl("resist"),
		"add-unit", "resist", "--num-units", "4",
	}
	c.Assert(commandmocker.Parameters(tmpdir), DeepEquals, expectedParams)
}

func (s *S) TestAddZeroUnits(c *C) {
	p := JujuProvisioner{}
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("headlong", "rush", 1)
	p := JujuProvisioner{}
	units, err := p.AddUnits(app, 1)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	e, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(e.Reason, Equals, "juju failed")
	c.Assert(e.Err.Error(), Equals, "exit status 1")
}

func (s *S) TestRemoveUnit(c *C) {
	tmpdir, err := commandmocker.Add("juju", "removed")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("two", "rush", 3)
	p := JujuProvisioner{}
	err = p.RemoveUnit(app, "two/2")
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	expected := []string{"remove-unit", "two/2", "terminate-machine", "3"}
	ran := make(chan bool, 1)
	go func() {
		for {
			if reflect.DeepEqual(commandmocker.Parameters(tmpdir), expected) {
				ran <- true
			}
		}
	}()
	select {
	case <-ran:
	case <-time.After(2e9):
		c.Errorf("Did not run terminate-machine command after 2 seconds.")
	}
}

func (s *S) TestRemoveUnknownUnit(c *C) {
	app := NewFakeApp("tears", "rush", 2)
	p := JujuProvisioner{}
	err := p.RemoveUnit(app, "tears/2")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `App "tears" does not have a unit named "tears/2".`)
}

func (s *S) TestRemoveUnitFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 66)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("something", "rush", 1)
	p := JujuProvisioner{}
	err = p.RemoveUnit(app, "something/0")
	c.Assert(err, NotNil)
	e, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(e.Reason, Equals, "juju failed")
	c.Assert(e.Err.Error(), Equals, "exit status 66")
}

func (s *S) TestRemoveUnits(c *C) {
	tmpdir, err := commandmocker.Add("juju", "removed")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("xanadu", "rush", 4)
	p := JujuProvisioner{}
	units, err := p.RemoveUnits(app, 3)
	c.Assert(err, IsNil)
	expected := []string{
		"remove-unit", "xanadu/0", "xanadu/1", "xanadu/2",
		"terminate-machine", "1",
		"terminate-machine", "2",
		"terminate-machine", "3",
	}
	ran := make(chan bool, 1)
	go func() {
		for {
			if reflect.DeepEqual(commandmocker.Parameters(tmpdir), expected) {
				ran <- true
			}
		}
	}()
	select {
	case <-ran:
	case <-time.After(2e9):
		params := commandmocker.Parameters(tmpdir)
		c.Fatalf("Did not run terminate-machine commands after 2 seconds. Parameters: %#v", params)
	}
	c.Assert(units, DeepEquals, []int{0, 1, 2})
}

func (s *S) TestRemoveAllUnits(c *C) {
	app := NewFakeApp("xanadu", "rush", 2)
	p := JujuProvisioner{}
	units, err := p.RemoveUnits(app, 2)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You can't remove all units from an app.")
}

func (s *S) TestRemoveTooManyUnits(c *C) {
	app := NewFakeApp("xanadu", "rush", 2)
	p := JujuProvisioner{}
	units, err := p.RemoveUnits(app, 3)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "You can't remove 3 units from this app because it has only 2 units.")
}

func (s *S) TestRemoveUnitsFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 66)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("closer", "rush", 3)
	p := JujuProvisioner{}
	units, err := p.RemoveUnits(app, 2)
	c.Assert(units, IsNil)
	c.Assert(err, NotNil)
	e, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(e.Reason, Equals, "juju failed")
	c.Assert(e.Err.Error(), Equals, "exit status 66")
}

func (s *S) TestExecuteCommand(c *C) {
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("almah", "static", 2)
	p := JujuProvisioner{}
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, IsNil)
	bufOutput := `Output from unit "almah/0":

ssh -o StrictHostKeyChecking no -q 1 ls -lh

Output from unit "almah/1":

ssh -o StrictHostKeyChecking no -q 2 ls -lh
`
	cmdOutput := "ssh -o StrictHostKeyChecking no -q 1 ls -lhssh -o StrictHostKeyChecking no -q 2 ls -lh"
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, cmdOutput)
	c.Assert(buf.String(), Equals, bufOutput)
}

func (s *S) TestExecuteCommandFailure(c *C) {
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Error("juju", "failed", 2)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("frases", "static", 1)
	p := JujuProvisioner{}
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-l")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "exit status 2")
	c.Assert(buf.String(), Equals, "failed\n")
}

func (s *S) TestExecuteCommandOneUnit(c *C) {
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("almah", "static", 1)
	p := JujuProvisioner{}
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, IsNil)
	output := "ssh -o StrictHostKeyChecking no -q 1 ls -lh"
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, output)
	c.Assert(buf.String(), Equals, output+"\n")
}

func (s *S) TestExecuteCommandUnitDown(c *C) {
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("almah", "static", 3)
	app.units[1].(*FakeUnit).status = provision.StatusDown
	p := JujuProvisioner{}
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-lha")
	c.Assert(err, IsNil)
	cmdOutput := "ssh -o StrictHostKeyChecking no -q 1 ls -lha"
	cmdOutput += "ssh -o StrictHostKeyChecking no -q 3 ls -lha"
	bufOutput := `Output from unit "almah/0":

ssh -o StrictHostKeyChecking no -q 1 ls -lha

Output from unit "almah/1":

Unit state is "down", it must be "started" for running commands.

Output from unit "almah/2":

ssh -o StrictHostKeyChecking no -q 3 ls -lha
`
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, cmdOutput)
	c.Assert(buf.String(), Equals, bufOutput)
}

func (s *S) TestCollectStatus(c *C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	expected := []provision.Unit{
		{
			Name:       "as_i_rise/0",
			AppName:    "as_i_rise",
			Type:       "django",
			Machine:    105,
			InstanceId: "i-00000439",
			Ip:         "10.10.10.163",
			Status:     provision.StatusStarted,
		},
		{
			Name:       "the_infanta/0",
			AppName:    "the_infanta",
			Type:       "gunicorn",
			Machine:    107,
			InstanceId: "i-0000043e",
			Ip:         "10.10.10.168",
			Status:     provision.StatusInstalling,
		},
	}
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	if units[0].Type == "gunicorn" {
		units[0], units[1] = units[1], units[0]
	}
	c.Assert(units, DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
}

func (s *S) TestCollectStatusDirtyOutput(c *C) {
	tmpdir, err := commandmocker.Add("juju", dirtyCollectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	expected := []provision.Unit{
		{
			Name:       "as_i_rise/0",
			AppName:    "as_i_rise",
			Type:       "django",
			Machine:    105,
			InstanceId: "i-00000439",
			Ip:         "10.10.10.163",
			Status:     provision.StatusStarted,
		},
		{
			Name:       "the_infanta/1",
			AppName:    "the_infanta",
			Type:       "gunicorn",
			Machine:    107,
			InstanceId: "i-0000043e",
			Ip:         "10.10.10.168",
			Status:     provision.StatusInstalling,
		},
	}
	p := JujuProvisioner{}
	units, err := p.CollectStatus()
	c.Assert(err, IsNil)
	if units[0].Type == "gunicorn" {
		units[0], units[1] = units[1], units[0]
	}
	c.Assert(units, DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
}

func (s *S) TestCollectStatusFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(pErr.Reason, Equals, "juju failed")
	c.Assert(pErr.Err.Error(), Equals, "exit status 1")
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
}

func (s *S) TestCollectStatusInvalidYAML(c *C) {
	tmpdir, err := commandmocker.Add("juju", "local: somewhere::")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, Equals, true)
	c.Assert(pErr.Reason, Equals, `"juju status" returned invalid data`)
	c.Assert(pErr.Err, ErrorMatches, `^YAML error:.*$`)
}

func (s *S) TestExecWithTimeout(c *C) {
	var data = []struct {
		cmd     []string
		timeout time.Duration
		out     string
		err     error
	}{
		{
			cmd:     []string{"sleep", "2"},
			timeout: 1e6,
			out:     "",
			err:     errors.New(`"sleep 2" ran for more than 1ms.`),
		},
		{
			cmd:     []string{"python", "-c", "import time; time.sleep(1); print 'hello world!'"},
			timeout: 5e9,
			out:     "hello world!\n",
			err:     nil,
		},
		{
			cmd:     []string{"python", "-c", "import sys; print 'hello world!'; exit(1)"},
			timeout: 5e9,
			out:     "hello world!\n",
			err:     errors.New("exit status 1"),
		},
	}
	for _, d := range data {
		out, err := execWithTimeout(d.timeout, d.cmd[0], d.cmd[1:]...)
		if string(out) != d.out {
			c.Errorf("Output. Want %q. Got %q.", d.out, out)
		}
		if d.err == nil && err != nil {
			c.Errorf("Error. Want %v. Got %v.", d.err, err)
		} else if d.err != nil && err.Error() != d.err.Error() {
			c.Errorf("Error message. Want %q. Got %q.", d.err.Error(), err.Error())
		}
	}
}

func (s *S) TestUnitStatus(c *C) {
	var tests = []struct {
		instance     string
		agent        string
		machineAgent string
		expected     provision.Status
	}{
		{"something", "nothing", "wut", provision.StatusPending},
		{"", "", "", provision.StatusCreating},
		{"", "", "pending", provision.StatusCreating},
		{"", "", "not-started", provision.StatusCreating},
		{"pending", "", "", provision.StatusCreating},
		{"", "not-started", "running", provision.StatusCreating},
		{"error", "install-error", "start-error", provision.StatusError},
		{"running", "pending", "running", provision.StatusInstalling},
		{"running", "started", "running", provision.StatusStarted},
		{"running", "down", "running", provision.StatusDown},
	}
	for _, t := range tests {
		got := unitStatus(t.instance, t.agent, t.machineAgent)
		if got != t.expected {
			c.Errorf("unitStatus(%q, %q, %q): Want %q. Got %q.", t.instance, t.agent, t.machineAgent, t.expected, got)
		}
	}
}
