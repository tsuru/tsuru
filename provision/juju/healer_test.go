// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"fmt"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/tsuru/tsuru/app"
	etesting "github.com/tsuru/tsuru/exec/testing"
	"github.com/tsuru/tsuru/heal"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/commandmocker"
	"github.com/tsuru/config"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3"
	"launchpad.net/goamz/s3/s3test"
	"launchpad.net/gocheck"
	"net"
)

func (s *S) TestBootstrapInstanceIDHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "bootstrap-instanceid")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, bootstrapInstanceIDHealer{})
}

func (s *S) TestBootstrapInstanceIDHealerNeedsHeal(c *gocheck.C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIDHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	err = bucket.Put("provider-state", []byte("doesnotexist"), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.needsHeal(), gocheck.Equals, true)
}

func (s *S) TestBootstrapInstanceIDHealerNotNeedsHeal(c *gocheck.C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIDHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	sg, err := h.ec2().CreateSecurityGroup("juju-delta-0", "")
	c.Assert(err, gocheck.IsNil)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1, SecurityGroups: []ec2.SecurityGroup{sg.SecurityGroup}})
	c.Assert(err, gocheck.IsNil)
	err = bucket.Put("provider-state", []byte(resp.Instances[0].InstanceId), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.needsHeal(), gocheck.Equals, false)
}

func (s *S) TestBootstrapInstanceIDHealerHeal(c *gocheck.C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, gocheck.IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIDHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	sg, err := h.ec2().CreateSecurityGroup("juju-delta-0", "")
	c.Assert(err, gocheck.IsNil)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1, SecurityGroups: []ec2.SecurityGroup{sg.SecurityGroup}})
	c.Assert(err, gocheck.IsNil)
	err = bucket.Put("provider-state", []byte("doesnotexist"), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.needsHeal(), gocheck.Equals, true)
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	data, err := bucket.Get("provider-state")
	expected := "zookeeper-instances: [" + resp.Instances[0].InstanceId + "]"
	c.Assert(string(data), gocheck.Equals, expected)
}

func (s *S) TestBootstrapInstanceIDHealerEC2(c *gocheck.C) {
	h := bootstrapInstanceIDHealer{}
	ec2 := h.ec2()
	c.Assert(ec2.EC2Endpoint, gocheck.Equals, "")
}

func (s *S) TestBootstrapInstanceIDHealerS3(c *gocheck.C) {
	h := bootstrapInstanceIDHealer{}
	s3 := h.s3()
	c.Assert(s3.Region, gocheck.DeepEquals, aws.USEast)
}

func (s *S) TestInstanceAgentsConfigHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "instance-agents-config")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, instanceAgentsConfigHealer{})
}

func (s *S) TestInstanceAgenstConfigHealerGetEC2(c *gocheck.C) {
	h := instanceAgentsConfigHealer{}
	ec2 := h.ec2()
	c.Assert(ec2.EC2Endpoint, gocheck.Equals, "")
}

func (s *S) TestInstanceAgenstConfigHealerHeal(c *gocheck.C) {
	server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer server.Quit()
	id := server.NewInstances(1, "small", "ami-123", ec2.InstanceState{Code: 16, Name: "running"}, nil)[0]
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    id,
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	a := app.App{
		Name:  "as_i_rise",
		Units: []app.Unit{{Name: "as_i_rise/0", State: "down", Ip: "server-1081.novalocal"}},
	}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": "as_i_rise"})
	sshTmpdir, err := commandmocker.Error("ssh", "", 1)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(sshTmpdir)
	h := instanceAgentsConfigHealer{}
	auth := aws.Auth{AccessKey: "access", SecretKey: "secret"}
	region := aws.SAEast
	region.EC2Endpoint = server.URL()
	h.e = ec2.New(auth, region)
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"grep",
		"i-0.internal.invalid",
		"/etc/init/juju-machine-agent.conf",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"sudo",
		"sed",
		"-i",
		"'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\"i-0.internal.invalid:2181\"/g'",
		"/etc/init/juju-machine-agent.conf",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"grep",
		"i-0.internal.invalid",
		"/etc/init/juju-as_i_rise-0.conf",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"sudo",
		"sed",
		"-i",
		"'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\"i-0.internal.invalid:2181\"/g'",
		"/etc/init/juju-as_i_rise-0.conf",
	}
	c.Assert(commandmocker.Ran(sshTmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), gocheck.DeepEquals, sshOutput)
}

func (s *S) TestInstanceAgenstConfigHealerHealAWSFailure(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-0800",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	a := app.App{
		Name:  "as_i_rise",
		Units: []app.Unit{{Name: "as_i_rise/0", State: "down", Ip: "server-1081.novalocal"}},
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": "as_i_rise"})
	h := instanceAgentsConfigHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestBootstrapPrivateDns(c *gocheck.C) {
	server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer server.Quit()
	h := instanceAgentsConfigHealer{}
	region := aws.SAEast
	region.EC2Endpoint = server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1})
	c.Assert(err, gocheck.IsNil)
	instance := resp.Instances[0]
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "running",
		IPAddress:     "localhost",
		InstanceID:    instance.InstanceId,
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	dns, err := h.bootstrapPrivateDns()
	c.Assert(err, gocheck.IsNil)
	c.Assert(dns, gocheck.Equals, instance.PrivateDNSName)
}

func (s *S) TestGetPrivateDns(c *gocheck.C) {
	server, err := ec2test.NewServer()
	c.Assert(err, gocheck.IsNil)
	defer server.Quit()
	h := instanceAgentsConfigHealer{}
	region := aws.SAEast
	region.EC2Endpoint = server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1})
	c.Assert(err, gocheck.IsNil)
	instance := resp.Instances[0]
	dns, err := h.getPrivateDns(instance.InstanceId)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dns, gocheck.Equals, instance.PrivateDNSName)
}

func (s *S) TestInstanceUnitShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "instance-unit")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, instanceUnitHealer{})
}

func (s *S) TestInstaceUnitHealWhenEverythingIsOk(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	a := []app.App{
		{Name: "as_i_rise", Units: []app.Unit{{Name: "as_i_rise/0", State: "started", Ip: "server-1081.novalocal"}}},
		{Name: "the_infanta", Units: []app.Unit{{Name: "the_infanta/0", State: "started", Ip: "server-1086.novalocal"}}},
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{"as_i_rise", "the_infanta"}}})
	h := instanceUnitHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestInstaceUnitHeal(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	a := app.App{
		Name:  "as_i_rise",
		Units: []app.Unit{{Name: "as_i_rise/0", State: "down", Ip: "server-1081.novalocal"}},
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": "as_i_rise"})
	h := instanceUnitHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"sudo",
		"stop",
		"juju-as_i_rise-0",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
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
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestInstanceMachineShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "instance-machine")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, instanceMachineHealer{})
}

func (s *S) TestInstanceMachineHealWhenEverythingIsOk(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	jujuTmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	h := instanceMachineHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), gocheck.DeepEquals, args)
}

func (s *S) TestInstanceMachineHeal(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputInstanceDown)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	h := instanceMachineHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"10.10.10.163",
		"sudo",
		"stop",
		"juju-machine-agent",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
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
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), gocheck.DeepEquals, args)
}

func (s *S) TestZookeeperHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "zookeeper")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, zookeeperHealer{})
}

func (s *S) TestZookeeperNeedsHeal(c *gocheck.C) {
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, gocheck.IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "notok")
		conn.Close()
	}()
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	h := zookeeperHealer{}
	c.Assert(h.needsHeal(), gocheck.Equals, true)
}

func (s *S) TestZookeeperNeedsHealConnectionRefused(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	h := zookeeperHealer{}
	c.Assert(h.needsHeal(), gocheck.Equals, true)
}

func (s *S) TestZookeeperNotNeedsHeal(c *gocheck.C) {
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, gocheck.IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "imok")
		conn.Close()
	}()
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	h := zookeeperHealer{}
	c.Assert(h.needsHeal(), gocheck.Equals, false)
}

func (s *S) TestZookeeperHealerHeal(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	ln, err := net.Listen("tcp", ":2181")
	c.Assert(err, gocheck.IsNil)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		fmt.Fprintln(conn, "notok")
		conn.Close()
	}()
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	h := zookeeperHealer{}
	err = h.Heal()
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"stop",
		"zookeeper",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
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
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestBootstrapProvisionHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "bootstrap-provision")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, bootstrapProvisionHealer{})
}

func (s *S) TestBootstrapProvisionHealerHeal(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	args := []string{
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
	err := h.Heal()
	c.Assert(err, gocheck.IsNil)
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestBootstrapMachineHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "bootstrap")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, bootstrapMachineHealer{})
}

func (s *S) TestBootstrapMachineHealerNeedsHeal(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	h := bootstrapMachineHealer{}
	c.Assert(h.needsHeal(), gocheck.Equals, true)
}

func (s *S) TestBootstrapMachineHealerDontNeedsHeal(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	h := bootstrapMachineHealer{}
	c.Assert(h.needsHeal(), gocheck.Equals, false)
}

func (s *S) TestBootstrapMachineHealerHeal(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	setExecut(fexec)
	defer setExecut(nil)
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "not-started",
		IPAddress:     "localhost",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	h := bootstrapMachineHealer{}
	err := h.Heal()
	c.Assert(err, gocheck.IsNil)
	args := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"localhost",
		"sudo",
		"stop",
		"juju-machine-agent",
	}
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
	args = []string{
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
	c.Assert(fexec.ExecutedCmd("ssh", args), gocheck.Equals, true)
}

func (s *S) TestBootstrapMachineHealerOnlyHealsWhenItIsNeeded(c *gocheck.C) {
	p := JujuProvisioner{}
	m := machine{
		AgentState:    "running",
		IPAddress:     "10.10.10.96",
		InstanceID:    "i-00000376",
		InstanceState: "running",
	}
	p.saveBootstrapMachine(m)
	collection := p.bootstrapCollection()
	defer collection.Close()
	defer collection.Remove(m)
	h := bootstrapMachineHealer{}
	err := h.Heal()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestELBInstanceHealerShouldBeRegistered(c *gocheck.C) {
	h, err := heal.Get("juju", "elb-instance")
	c.Assert(err, gocheck.IsNil)
	c.Assert(h, gocheck.FitsTypeOf, elbInstanceHealer{})
}

func (s *S) TestELBInstanceHealerCheckInstancesDisabledELB(c *gocheck.C) {
	healer := elbInstanceHealer{}
	instances, err := healer.checkInstances(nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(instances, gocheck.HasLen, 0)
}

func (s *ELBSuite) TestELBInstanceHealerGetUnhealthyApps(c *gocheck.C) {
	apps := []interface{}{
		app.App{Name: "when", Units: []app.Unit{
			{Name: "when/0", State: provision.StatusStarted.String()},
		}},
		app.App{Name: "what", Units: []app.Unit{
			{Name: "what/0", State: provision.StatusDown.String()},
		}},
		app.App{Name: "why", Units: []app.Unit{
			{Name: "why/0", State: provision.StatusDown.String()},
		}},
		app.App{Name: "how", Units: []app.Unit{
			{Name: "how/0", State: provision.StatusStarted.String()},
			{Name: "how/1", State: provision.StatusDown.String()},
		}},
	}
	err := s.conn.Apps().Insert(apps...)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{"what", "when", "why", "how"}}})
	healer := elbInstanceHealer{}
	unhealthy := healer.getUnhealthyApps()
	expected := map[string]app.App{
		"what": apps[1].(app.App),
		"why":  apps[2].(app.App),
		"how":  apps[3].(app.App),
	}
	c.Assert(unhealthy, gocheck.DeepEquals, expected)
}

func (s *ELBSuite) TestELBInstanceHealerCheckInstances(c *gocheck.C) {
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
	healer := elbInstanceHealer{}
	instances, err := healer.checkInstances([]string{lb})
	c.Assert(err, gocheck.IsNil)
	expected := []elbInstance{
		{
			description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
			state:       "OutOfService",
			reasonCode:  "Instance",
			id:          instance,
			lb:          "elbtest",
		},
	}
	c.Assert(instances, gocheck.DeepEquals, expected)
}
