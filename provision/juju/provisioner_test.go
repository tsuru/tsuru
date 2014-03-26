// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"errors"
	"github.com/globocom/tsuru/exec"
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/testing"
	"github.com/tsuru/commandmocker"
	"github.com/tsuru/config"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"runtime"
	"strconv"
	"time"
)

func setExecut(e exec.Executor) {
	execMut.Lock()
	execut = e
	execMut.Unlock()
}

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	p, err := provision.Get("juju")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.FitsTypeOf, &JujuProvisioner{})
}

func (s *S) TestELBSupport(c *gocheck.C) {
	defer config.Unset("juju:use-elb")
	config.Set("juju:use-elb", true)
	p := JujuProvisioner{}
	c.Assert(p.elbSupport(), gocheck.Equals, true)
	config.Set("juju:use-elb", false)
	c.Assert(p.elbSupport(), gocheck.Equals, true) // Read config only once.
	p = JujuProvisioner{}
	c.Assert(p.elbSupport(), gocheck.Equals, false)
	config.Unset("juju:use-elb")
	p = JujuProvisioner{}
	c.Assert(p.elbSupport(), gocheck.Equals, false)
}

func (s *S) TestUnitsCollection(c *gocheck.C) {
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}

func (s *S) TestProvision(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	config.Set("juju:charms-path", "/etc/juju/charms")
	defer config.Unset("juju:charms-path")
	config.Set("host", "somehost")
	defer config.Unset("host")
	app := testing.NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"deploy", "--repository", "/etc/juju/charms", "local:python", "trace",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{
		"set", "trace", "app-repo=" + repository.ReadOnlyURL("trace"),
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestProvisionUndefinedCharmsPath(c *gocheck.C) {
	config.Unset("juju:charms-path")
	p := JujuProvisioner{}
	err := p.Provision(testing.NewFakeApp("eternity", "sandman", 0))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "juju:charms-path" is not defined.`)
}

func (s *S) TestProvisionFailure(c *gocheck.C) {
	config.Set("juju:charms-path", "/home/charms")
	defer config.Unset("juju:charms-path")
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	err = p.Provision(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 1")
}

func (s *S) TestRestart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := JujuProvisioner{}
	err := p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh", "-o", "StrictHostKeyChecking no", "-q", "1", "/var/lib/tsuru/hooks/restart",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestRestartFailure(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	tmpdir, err := commandmocker.Error("juju", "juju failed to run command", 25)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := JujuProvisioner{}
	err = p.Restart(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed to run command\n")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 25")
}

func (s *S) TestStop(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("app", "python", 1)
	p := JujuProvisioner{}
	err := p.Stop(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh", "-o", "StrictHostKeyChecking no", "-q", "1", "/var/lib/tsuru/hooks/stop",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestDeploy(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", "")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	config.Set("git:unit-repo", "test/dir")
	defer func() {
		config.Unset("git:unit-repo")
	}()
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	p := JujuProvisioner{}
	err = p.Deploy(app, "f83ac40", w)
	c.Assert(err, gocheck.IsNil)
	expected := []string{"set", app.GetName(), "app-version=f83ac40"}
	c.Assert(commandmocker.Parameters(tmpdir)[:3], gocheck.DeepEquals, expected)
}

func (s *S) TestDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("cribcaged", "python", 3)
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	err := collection.Insert(
		instance{UnitName: "cribcaged/0"},
		instance{UnitName: "cribcaged/1"},
		instance{UnitName: "cribcaged/2"},
	)
	c.Assert(err, gocheck.IsNil)
	err = p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{"destroy-service", "cribcaged"}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{"terminate-machine", "1"}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{"terminate-machine", "2"}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{"terminate-machine", "3"}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	n, err := collection.Find(bson.M{
		"_id": bson.M{
			"$in": []string{"cribcaged/0", "cribcaged/1", "cribcaged/2"},
		},
	}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestDestroyFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed to destroy the machine", 25)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("idioglossia", "static", 1)
	p := JujuProvisioner{}
	err = p.Destroy(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed to destroy the machine")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 25")
}

func (s *S) TestAddUnits(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", addUnitsOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("resist", "rush", 0)
	p := JujuProvisioner{}
	units, err := p.AddUnits(app, 4)
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.HasLen, 4)
	names := make([]string, len(units))
	for i, unit := range units {
		names[i] = unit.Name
	}
	expected := []string{"resist/3", "resist/4", "resist/5", "resist/6"}
	c.Assert(names, gocheck.DeepEquals, expected)
	args := []string{
		"add-unit", "resist", "--num-units", "4",
	}
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, args)
	_, err = getQueue(queueName).Get(1e6)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddZeroUnits(c *gocheck.C) {
	p := JujuProvisioner{}
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("headlong", "rush", 1)
	p := JujuProvisioner{}
	units, err := p.AddUnits(app, 1)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Reason, gocheck.Equals, "juju failed")
	c.Assert(e.Err.Error(), gocheck.Equals, "exit status 1")
}

func (s *S) TestRemoveUnit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("two", "rush", 3)
	p := JujuProvisioner{}
	collection := p.unitsCollection()
	defer collection.Close()
	err := collection.Insert(instance{UnitName: "two/2", InstanceID: "i-00000439"})
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnit(app, "two/2")
	c.Assert(err, gocheck.IsNil)
	ran := make(chan bool, 1)
	go func() {
		for {
			args1 := []string{"remove-unit", "two/2"}
			args2 := []string{"terminate-machine", "3"}
			if fexec.ExecutedCmd("juju", args1) && fexec.ExecutedCmd("juju", args2) {
				ran <- true
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ran:
	case <-time.After(2e9):
		c.Errorf("Did not run terminate-machine command after 2 seconds.")
	}
	n, err := collection.Find(bson.M{"_id": "two/2"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *S) TestRemoveUnitUnknownByJuju(c *gocheck.C) {
	output := `013-01-11 20:02:07,883 INFO Connecting to environment...
2013-01-11 20:02:10,147 INFO Connected to environment.
2013-01-11 20:02:10,160 ERROR Service unit 'two/2' was not found`
	tmpdir, err := commandmocker.Error("juju", output, 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("two", "rush", 3)
	p := JujuProvisioner{}
	err = p.RemoveUnit(app, "two/2")
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestRemoveUnknownUnit(c *gocheck.C) {
	app := testing.NewFakeApp("tears", "rush", 2)
	p := JujuProvisioner{}
	err := p.RemoveUnit(app, "tears/2")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `App "tears" does not have a unit named "tears/2".`)
}

func (s *S) TestRemoveUnitFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 66)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("something", "rush", 1)
	p := JujuProvisioner{}
	err = p.RemoveUnit(app, "something/0")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Reason, gocheck.Equals, "juju failed")
	c.Assert(e.Err.Error(), gocheck.Equals, "exit status 66")
}

func (s *S) TestInstallDepsRunRelatedHook(c *gocheck.C) {
	p := &JujuProvisioner{}
	app := testing.NewFakeApp("myapp", "python", 0)
	w := &bytes.Buffer{}
	err := p.InstallDeps(app, w)
	c.Assert(err, gocheck.IsNil)
	expected := []string{"ran /var/lib/tsuru/hooks/dependencies"}
	c.Assert(app.Commands, gocheck.DeepEquals, expected)
}

func (s *S) TestExecutedCmd(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("almah", "static", 2)
	p := JujuProvisioner{}
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	bufOutput := `Output from unit "almah/0":



Output from unit "almah/1":


`
	args := []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"1",
		"ls",
		"-lh",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"2",
		"ls",
		"-lh",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	c.Assert(buf.String(), gocheck.Equals, bufOutput)
}

func (s *S) TestExecutedCmdFailure(c *gocheck.C) {
	var buf bytes.Buffer
	tmpdir, err := commandmocker.Error("juju", "failed", 2)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("frases", "static", 1)
	p := JujuProvisioner{}
	err = p.ExecuteCommand(&buf, &buf, app, "ls", "-l")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "exit status 2")
	c.Assert(buf.String(), gocheck.Equals, "failed\n")
}

func (s *S) TestExecutedCmdOneUnit(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("almah", "static", 1)
	p := JujuProvisioner{}
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"1",
		"ls",
		"-lh",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestExecutedCmdUnitDown(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("almah", "static", 3)
	app.SetUnitStatus(provision.StatusDown, 1)
	p := JujuProvisioner{}
	err := p.ExecuteCommand(&buf, &buf, app, "ls", "-lha")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"1",
		"ls",
		"-lha",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	args = []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"3",
		"ls",
		"-lha",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	bufOutput := `Output from unit "almah/0":



Output from unit "almah/2":


`
	c.Assert(buf.String(), gocheck.Equals, bufOutput)
}

func (s *S) TestExecWithTimeout(c *gocheck.C) {
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
			cmd:     []string{"python", "-c", "import time; time.sleep(1); print('hello world!')"},
			timeout: 5e9,
			out:     "hello world!\n",
			err:     nil,
		},
		{
			cmd:     []string{"python", "-c", "import sys; print('hello world!'); exit(1)"},
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

func (s *S) TestAddr(c *gocheck.C) {
	app := testing.NewFakeApp("blue", "who", 1)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, app.ProvisionedUnits()[0].GetIp())
}

func (s *S) TestAddrWithoutUnits(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	app := testing.NewFakeApp("squeeze", "who", 0)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `App "squeeze" has no units.`)
}

func (s *ELBSuite) TestProvisionWithELB(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	config.Set("juju:charms-path", "/home/charms")
	defer config.Unset("juju:charms-path")
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(app.GetName())
	addr, err := router.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Not(gocheck.Equals), "")
	msg, err := getQueue(queueName).Get(1e9)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Action, gocheck.Equals, addUnitToLoadBalancer)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{"jimmy"})
}

func (s *ELBSuite) TestDestroyWithELB(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	config.Set("juju:charms-path", "/home/charms")
	defer config.Unset("juju:charms-path")
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(app.GetName())
	addr, err := router.Addr(app.GetName())
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "not found")
	q := getQueue(queueName)
	msg, err := q.Get(1e9)
	c.Assert(err, gocheck.IsNil)
	if msg.Action != addUnitToLoadBalancer {
		q.Put(msg, 0)
	}
}

func (s *ELBSuite) TestAddUnitsWithELB(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", addUnitsOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("resist", "rush", 0)
	p := JujuProvisioner{}
	_, err = p.AddUnits(app, 4)
	c.Assert(err, gocheck.IsNil)
	expected := []string{
		"resist", "resist/3", "resist/4",
		"resist/5", "resist/6",
	}
	msg, err := getQueue(queueName).Get(1e9)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Action, gocheck.Equals, addUnitToLoadBalancer)
	c.Assert(msg.Args, gocheck.DeepEquals, expected)
}

func (s *ELBSuite) TestRemoveUnitWithELB(c *gocheck.C) {
	instIds := make([]string, 4)
	units := make([]provision.Unit, len(instIds))
	for i := 0; i < len(instIds); i++ {
		id := s.server.NewInstance()
		defer s.server.RemoveInstance(id)
		instIds[i] = id
		units[i] = provision.Unit{
			Name:       "radio/" + strconv.Itoa(i),
			InstanceId: id,
		}
	}
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("radio", "rush", 4)
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	err = router.AddBackend(app.GetName())
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(app.GetName())
	for _, unit := range units {
		err = router.AddRoute(app.GetName(), unit.InstanceId)
		c.Assert(err, gocheck.IsNil)
	}
	p := JujuProvisioner{}
	fUnit := testing.FakeUnit{Name: units[0].Name, InstanceId: units[0].InstanceId}
	err = p.removeUnit(app, &fUnit)
	c.Assert(err, gocheck.IsNil)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, gocheck.HasLen, len(units)-1)
	instance := resp.LoadBalancerDescriptions[0].Instances[0]
	c.Assert(instance.InstanceId, gocheck.Equals, instIds[1])
}

func (s *ELBSuite) TestAddrWithELB(c *gocheck.C) {
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	router.AddBackend(app.GetName())
	defer router.RemoveBackend(app.GetName())
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	elbAddr, err := router.Addr(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, elbAddr)
}

func (s *ELBSuite) TestAddrWithUnknownELB(c *gocheck.C) {
	app := testing.NewFakeApp("jimmi", "who", 0)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^There is no ACTIVE Load Balancer named.*")
}

func (s *ELBSuite) TestSwap(c *gocheck.C) {
	var p JujuProvisioner
	app1 := testing.NewFakeApp("app1", "python", 1)
	app2 := testing.NewFakeApp("app2", "python", 1)
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	router, err := Router()
	c.Assert(err, gocheck.IsNil)
	err = router.AddBackend(app1.GetName())
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(app1.GetName())
	err = router.AddRoute(app1.GetName(), id1)
	c.Assert(err, gocheck.IsNil)
	err = router.AddBackend(app2.GetName())
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend(app2.GetName())
	err = router.AddRoute(app2.GetName(), id2)
	c.Assert(err, gocheck.IsNil)
	err = p.Swap(app1, app2)
	c.Assert(err, gocheck.IsNil)
	app2Routes, err := router.Routes(app2.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(app2Routes, gocheck.DeepEquals, []string{id2})
	app1Routes, err := router.Routes(app1.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(app1Routes, gocheck.DeepEquals, []string{id1})
	addr, err := router.Addr(app1.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "app2-some-aws-stuff.us-east-1.elb.amazonaws.com")
	addr, err = router.Addr(app2.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "app1-some-aws-stuff.us-east-1.elb.amazonaws.com")
}

func (s *S) TestExecutedCommandOnce(c *gocheck.C) {
	var buf bytes.Buffer
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("almah", "static", 2)
	p := JujuProvisioner{}
	err := p.ExecuteCommandOnce(&buf, &buf, app, "ls", "-lh")
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"1",
		"ls",
		"-lh",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
	c.Assert(buf.String(), gocheck.Equals, "\n")
}

func (s *S) TestStartedUnits(c *gocheck.C) {
	app := testing.NewFakeApp("almah", "static", 2)
	app.SetUnitStatus(provision.StatusDown, 1)
	p := JujuProvisioner{}
	units := p.startedUnits(app)
	c.Assert(units, gocheck.HasLen, 1)
}

func (s *S) TestStartedUnitsShouldReturnTrueForUnreachable(c *gocheck.C) {
	app := testing.NewFakeApp("almah", "static", 1)
	app.SetUnitStatus(provision.StatusUnreachable, 0)
	p := JujuProvisioner{}
	units := p.startedUnits(app)
	c.Assert(units, gocheck.HasLen, 1)
}

func (s *S) TestDeployPipeline(c *gocheck.C) {
	p := JujuProvisioner{}
	c.Assert(p.DeployPipeline(), gocheck.IsNil)
}

func (s *S) TestStart(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := JujuProvisioner{}
	err := p.Start(app)
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"ssh", "-o", "StrictHostKeyChecking no", "-q", "1", "/var/lib/tsuru/hooks/start",
	}
	c.Assert(fexec.ExecutedCmd("juju", args), gocheck.Equals, true)
}

func (s *S) TestStartFailure(c *gocheck.C) {
	// h := &testing.TestHandler{}
	// gandalfServer := testing.StartGandalfTestServer(h)
	// defer gandalfServer.Close()
	tmpdir, err := commandmocker.Error("juju", "juju failed to run command", 25)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("cribcaged", "python", 1)
	p := JujuProvisioner{}
	err = p.Start(app)
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed to run command\n")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 25")
}
