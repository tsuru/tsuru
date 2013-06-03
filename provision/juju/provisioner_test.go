// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"errors"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
	conn, collection := p.unitsCollection()
	defer conn.Close()
	c.Assert(collection.Name, gocheck.Equals, s.collName)
}

func (s *S) TestProvision(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	err = p.Deploy(app, w)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("cribcaged", "python", 3)
	p := JujuProvisioner{}
	conn, collection := p.unitsCollection()
	defer conn.Close()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("two", "rush", 3)
	p := JujuProvisioner{}
	conn, collection := p.unitsCollection()
	defer conn.Close()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
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
	execut = fexec
	defer func() {
		execut = nil
	}()
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



Output from unit "almah/1":

Unit state is "down", it must be "started" for running commands.

Output from unit "almah/2":


`
	c.Assert(buf.String(), gocheck.Equals, bufOutput)
}

func (s *S) TestSaveBootstrapMachine(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "state",
		IPAddress:     "ip",
		InstanceID:    "id",
		InstanceState: "state",
	}
	p.saveBootstrapMachine(m)
	conn, collection := p.bootstrapCollection()
	defer conn.Close()
	defer collection.Remove(m)
	var mach machine
	collection.Find(nil).One(&mach)
	c.Assert(mach, gocheck.DeepEquals, m)
}

func (s *S) TestCollectStatusShouldNotAddBootstraTwice(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	conn, collection := p.bootstrapCollection()
	defer conn.Close()
	l, err := collection.Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(l, gocheck.Equals, 1)
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	conn, collection := p.unitsCollection()
	defer conn.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "i-00000439"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
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
	c.Assert(err, gocheck.IsNil)
	cp := make([]provision.Unit, len(units))
	copy(cp, units)
	if cp[0].Type == "gunicorn" {
		cp[0], cp[1] = cp[1], cp[0]
	}
	c.Assert(cp, gocheck.DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	done := make(chan int8)
	go func() {
		for {
			ct, err := collection.Find(nil).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 2 {
				done <- 1
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not save the unit after 5 seconds.")
	}
	var instances []instance
	err = collection.Find(nil).Sort("_id").All(&instances)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instances, gocheck.HasLen, 2)
	c.Assert(instances[0].UnitName, gocheck.Equals, "as_i_rise/0")
	c.Assert(instances[0].InstanceID, gocheck.Equals, "i-00000439")
	c.Assert(instances[1].UnitName, gocheck.Equals, "the_infanta/0")
	c.Assert(instances[1].InstanceID, gocheck.Equals, "i-0000043e")
	var b machine
	err = collection.Find(nil).One(&b)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCollectStatusDirtyOutput(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", dirtyCollectOutput)
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(err, gocheck.IsNil)
	cp := make([]provision.Unit, len(units))
	copy(cp, units)
	if cp[0].Type == "gunicorn" {
		cp[0], cp[1] = cp[1], cp[0]
	}
	c.Assert(cp, gocheck.DeepEquals, expected)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	var wg sync.WaitGroup
	wg.Add(1)
	conn, collection := p.unitsCollection()
	defer conn.Close()
	go func() {
		q := bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/1"}}}
		for {
			if n, _ := collection.Find(q).Count(); n == 2 {
				break
			}
			runtime.Gosched()
		}
		collection.Remove(q)
		wg.Done()
	}()
	wg.Wait()
}

func (s *S) TestCollectStatusIDChangeDisabledELB(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	conn, collection := p.unitsCollection()
	defer conn.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "i-00000239"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "as_i_rise/0", "instanceid": "i-00000439"}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not update the unit after 5 seconds.")
	}
}

func (s *S) TestCollectStatusIDChangeFromPending(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	conn, collection := p.unitsCollection()
	defer conn.Close()
	err = collection.Insert(instance{UnitName: "as_i_rise/0", InstanceID: "pending"})
	c.Assert(err, gocheck.IsNil)
	defer collection.Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "as_i_rise/0", "instanceid": "i-00000439"}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not update the unit after 5 seconds.")
	}
}

func (s *S) TestCollectStatusFailure(c *gocheck.C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, "juju failed")
	c.Assert(pErr.Err.Error(), gocheck.Equals, "exit status 1")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *S) TestCollectStatusInvalidYAML(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", "local: somewhere::")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.NotNil)
	pErr, ok := err.(*provision.Error)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(pErr.Reason, gocheck.Equals, `"juju status" returned invalid data: local: somewhere::`)
	c.Assert(pErr.Err, gocheck.ErrorMatches, `^YAML error:.*$`)
}

func (s *S) TestLoadBalancerEnabledElb(c *gocheck.C) {
	p := JujuProvisioner{}
	p.elb = new(bool)
	*p.elb = true
	lb := p.LoadBalancer()
	c.Assert(lb, gocheck.NotNil)
}

func (s *S) TestLoadBalancerDisabledElb(c *gocheck.C) {
	p := JujuProvisioner{}
	p.elb = new(bool)
	lb := p.LoadBalancer()
	c.Assert(lb, gocheck.IsNil)
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

func (s *S) TestUnitStatus(c *gocheck.C) {
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
		{"started", "start-error", "running", provision.StatusError},
		{"started", "charm-upgrade-error", "running", provision.StatusError},
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

func (s *S) TestAddr(c *gocheck.C) {
	app := testing.NewFakeApp("blue", "who", 1)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, app.ProvisionUnits()[0].GetIp())
}

func (s *S) TestAddrWithoutUnits(c *gocheck.C) {
	app := testing.NewFakeApp("squeeze", "who", 0)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `App "squeeze" has no units.`)
}

func (s *S) TestSetCName(c *gocheck.C) {
	app := testing.NewFakeApp("squeeze", "who", 0)
	err := (&JujuProvisioner{}).SetCName(app, "somecname.com")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	app := testing.NewFakeApp("squeeze", "who", 0)
	err := (&JujuProvisioner{}).UnsetCName(app, "somecname.com")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestProvisionerIsCNameManager(c *gocheck.C) {
	var p interface{}
	p = &JujuProvisioner{}
	_, ok := p.(provision.CNameManager)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *ELBSuite) TestProvisionWithELB(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	config.Set("juju:charms-path", "/home/charms")
	defer config.Unset("juju:charms-path")
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	lb := p.LoadBalancer()
	defer lb.Destroy(app)
	addr, err := lb.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Not(gocheck.Equals), "")
	msg, err := getQueue(queueName).Get(1e9)
	c.Assert(err, gocheck.IsNil)
	defer msg.Delete()
	c.Assert(msg.Action, gocheck.Equals, addUnitToLoadBalancer)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{"jimmy"})
}

func (s *ELBSuite) TestDestroyWithELB(c *gocheck.C) {
	config.Set("juju:charms-path", "/home/charms")
	defer config.Unset("juju:charms-path")
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	lb := p.LoadBalancer()
	defer lb.Destroy(app) // sanity
	addr, err := lb.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
	q := getQueue(queueName)
	msg, err := q.Get(1e9)
	c.Assert(err, gocheck.IsNil)
	if msg.Action == addUnitToLoadBalancer && msg.Args[0] == "jimmy" {
		msg.Delete()
	} else {
		q.Release(msg, 0)
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
	defer msg.Delete()
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
	execMut.Lock()
	execut = fexec
	execMut.Unlock()
	defer func() {
		execMut.Lock()
		defer execMut.Unlock()
		execut = nil
	}()
	app := testing.NewFakeApp("radio", "rush", 4)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	err = manager.Register(app, units...)
	c.Assert(err, gocheck.IsNil)
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

func (s *ELBSuite) TestCollectStatusWithELBAndIDChange(c *gocheck.C) {
	a := testing.NewFakeApp("symfonia", "symfonia", 0)
	p := JujuProvisioner{}
	lb := p.LoadBalancer()
	err := lb.Create(a)
	c.Assert(err, gocheck.IsNil)
	defer lb.Destroy(a)
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	id3 := s.server.NewInstance()
	defer s.server.RemoveInstance(id3)
	conn, collection := p.unitsCollection()
	defer conn.Close()
	err = collection.Insert(instance{UnitName: "symfonia/0", InstanceID: id3})
	c.Assert(err, gocheck.IsNil)
	err = lb.Register(a, provision.Unit{InstanceId: id3}, provision.Unit{InstanceId: id2})
	q := bson.M{"_id": bson.M{"$in": []string{"symfonia/0", "symfonia/1", "symfonia/2", "raise/0"}}}
	defer collection.Remove(q)
	output := strings.Replace(simpleCollectOutput, "i-00004444", id1, 1)
	output = strings.Replace(output, "i-00004445", id2, 1)
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	_, err = p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "symfonia/0", "instanceid": id1}
			ct, err := collection.Find(q).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 1 {
				done <- 1
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(5e9):
		c.Fatal("Did not save the unit after 5 seconds.")
	}
	resp, err := s.client.DescribeLoadBalancers(a.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 2)
	c.Assert(instances[0].InstanceId, gocheck.Equals, id2)
	c.Assert(instances[1].InstanceId, gocheck.Equals, id1)
}

func (s *ELBSuite) TestAddrWithELB(c *gocheck.C) {
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	lb := p.LoadBalancer()
	err := lb.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer lb.Destroy(app)
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	lAddr, err := lb.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, lAddr)
}

func (s *ELBSuite) TestAddrWithUnknownELB(c *gocheck.C) {
	app := testing.NewFakeApp("jimmy", "who", 0)
	p := JujuProvisioner{}
	addr, err := p.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}
