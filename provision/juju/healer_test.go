// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"fmt"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/heal"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net"
)

func (s *S) TestInstanceUnitShouldBeRegistered(c *C) {
	h, err := heal.Get("instance-unit")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &instanceUnitHealer{})
}

func (s *S) TestInstaceUnitHealWhenEverythingIsOk(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	h := instanceUnitHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, false)
}

func (s *S) TestInstaceUnitHeal(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputInstanceDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"sudo",
		"stop",
		"juju-as_i_rise-0",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"sudo",
		"start",
		"juju-as_i_rise-0",
	}
	h := instanceUnitHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestInstanceMachineShouldBeRegistered(c *C) {
	h, err := heal.Get("instance-machine")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &instanceMachineHealer{})
}

func (s *S) TestInstanceMachineHealWhenEverythingIsOk(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	h := instanceMachineHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, false)
}

func (s *S) TestInstanceMachineHeal(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputInstanceDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"10.10.10.163",
		"sudo",
		"stop",
		"juju-machine-agent",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"10.10.10.163",
		"sudo",
		"start",
		"juju-machine-agent",
	}
	h := instanceMachineHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestZookeeperHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("zookeeper")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &ZookeeperHealer{})
}

func (s *S) TestZookeeperNeedsHeal(c *C) {
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "notok")
		conn.Close()
	}()
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	h := ZookeeperHealer{}
	c.Assert(h.needsHeal(), Equals, true)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
}

func (s *S) TestZookeeperNeedsHealConnectionRefused(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	h := ZookeeperHealer{}
	c.Assert(h.needsHeal(), Equals, true)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
}

func (s *S) TestZookeeperNotNeedsHeal(c *C) {
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "imok")
		conn.Close()
	}()
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	h := ZookeeperHealer{}
	c.Assert(h.needsHeal(), Equals, false)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
}

func (s *S) TestZookeeperHealerHeal(c *C) {
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "notok")
		conn.Close()
	}()
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
		"status", // for juju status that gets the output
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"stop",
		"zookeeper",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"start",
		"zookeeper",
	}
	h := ZookeeperHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestBootstrapProvisionHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("bootstrap-provision")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &bootstrapProvisionHealer{})
}

func (s *S) TestBootstrapProvisionHealerHeal(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"start",
		"juju-provision-agent",
	}
	h := bootstrapProvisionHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestBootstrapMachineHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("bootstrap")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &bootstrapMachineHealer{})
}

func (s *S) TestBootstrapMachineHealerNeedsHeal(c *C) {
	tmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	h := bootstrapMachineHealer{}
	c.Assert(h.needsHeal(), Equals, true)
}

func (s *S) TestBootstrapMachineHealerDontNeedsHeal(c *C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	h := bootstrapMachineHealer{}
	c.Assert(h.needsHeal(), Equals, false)
}

func (s *S) TestBootstrapMachineHealerHeal(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Add("ssh", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	jujuOutput := []string{
		"status", // for verify if heal is need
		"status", // for juju status that gets the output
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"stop",
		"juju-machine-agent",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"start",
		"juju-machine-agent",
	}
	h := bootstrapMachineHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestBootstrapMachineHealerOnlyHealsWhenItIsNeeded(c *C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	cmdOutput := []string{
		"status", // for verify if heal is need
	}
	h := bootstrapMachineHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(tmpdir), DeepEquals, cmdOutput)
}

func (s *S) TestELBInstanceHealerCheckInstancesDisabledELB(c *C) {
	healer := ELBInstanceHealer{}
	instances, err := healer.checkInstances()
	c.Assert(err, IsNil)
	c.Assert(instances, HasLen, 0)
}

func (s *ELBSuite) TestELBInstanceHealerCheckInstances(c *C) {
	lb := "elbtest"
	instance := s.server.NewInstance()
	defer s.server.RemoveInstance(instance)
	s.server.NewLoadBalancer(lb)
	defer s.server.RemoveLoadBalancer(lb)
	s.server.RegisterInstance(instance, lb)
	defer s.server.DeregisterInstance(instance, lb)
	state := elb.InstanceState{
		Description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
		State:       "OutOfService",
		ReasonCode:  "Instance",
		InstanceId:  instance,
	}
	s.server.ChangeInstanceState(lb, state)
	healer := ELBInstanceHealer{}
	instances, err := healer.checkInstances()
	c.Assert(err, IsNil)
	expected := []elbInstance{
		{
			description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
			state:       "OutOfService",
			reasonCode:  "Instance",
			instanceId:  instance,
			lb:          "elbtest",
		},
	}
	c.Assert(instances, DeepEquals, expected)
}

func (s *ELBSuite) TestELBInstanceHealer(c *C) {
	lb := "elbtest"
	instance := s.server.NewInstance()
	defer s.server.RemoveInstance(instance)
	s.server.NewLoadBalancer(lb)
	defer s.server.RemoveLoadBalancer(lb)
	s.server.RegisterInstance(instance, lb)
	defer s.server.DeregisterInstance(instance, lb)
	a := app.App{
		Name:  "elbtest",
		Units: []app.Unit{{InstanceId: instance, State: "started", Name: "elbtest/0"}},
	}
	storage, err := db.Conn()
	c.Assert(err, IsNil)
	err = storage.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer storage.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	state := elb.InstanceState{
		Description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
		State:       "OutOfService",
		ReasonCode:  "Instance",
		InstanceId:  instance,
	}
	s.server.ChangeInstanceState(lb, state)
	healer := ELBInstanceHealer{}
	err = healer.Heal()
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.Units, HasLen, 1)
	c.Assert(a.Units[0].InstanceId, Not(Equals), instance)
}
