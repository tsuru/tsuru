// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"fmt"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/heal"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	"launchpad.net/goamz/s3"
	"launchpad.net/goamz/s3/s3test"
	. "launchpad.net/gocheck"
	"net"
)

func (s *S) TestBootstrapInstanceIdHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("bootstrap-instanceid")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, bootstrapInstanceIdHealer{})
}

func (s *S) TestBootstrapInstanceIdHealerNeedsHeal(c *C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIdHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, IsNil)
	err = bucket.Put("provider-state", []byte("doesnotexist"), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, IsNil)
	c.Assert(h.needsHeal(), Equals, true)
}

func (s *S) TestBootstrapInstanceIdHealerNotNeedsHeal(c *C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIdHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	sg, err := h.ec2().CreateSecurityGroup("juju-delta-0", "")
	c.Assert(err, IsNil)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, IsNil)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1, SecurityGroups: []ec2.SecurityGroup{sg.SecurityGroup}})
	c.Assert(err, IsNil)
	err = bucket.Put("provider-state", []byte(resp.Instances[0].InstanceId), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, IsNil)
	c.Assert(h.needsHeal(), Equals, false)
}

func (s *S) TestBootstrapInstanceIdHealerHeal(c *C) {
	ec2Server, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer ec2Server.Quit()
	s3Server, err := s3test.NewServer(nil)
	c.Assert(err, IsNil)
	defer s3Server.Quit()
	h := bootstrapInstanceIdHealer{}
	region := aws.SAEast
	region.EC2Endpoint = ec2Server.URL()
	region.S3Endpoint = s3Server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	sg, err := h.ec2().CreateSecurityGroup("juju-delta-0", "")
	c.Assert(err, IsNil)
	h.s = s3.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	jujuBucket := "ble"
	config.Set("juju:bucket", jujuBucket)
	bucket := h.s3().Bucket(jujuBucket)
	err = bucket.PutBucket(s3.PublicReadWrite)
	c.Assert(err, IsNil)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1, SecurityGroups: []ec2.SecurityGroup{sg.SecurityGroup}})
	c.Assert(err, IsNil)
	err = bucket.Put("provider-state", []byte("doesnotexist"), "binary/octet-stream", s3.PublicReadWrite)
	c.Assert(err, IsNil)
	c.Assert(h.needsHeal(), Equals, true)
	err = h.Heal()
	c.Assert(err, IsNil)
	data, err := bucket.Get("provider-state")
	expected := "zookeeper-instances: [" + resp.Instances[0].InstanceId + "]"
	c.Assert(string(data), Equals, expected)
}

func (s *S) TestBootstrapInstanceIdHealerEC2(c *C) {
	h := bootstrapInstanceIdHealer{}
	ec2 := h.ec2()
	c.Assert(ec2.EC2Endpoint, Equals, "")
}

func (s *S) TestBootstrapInstanceIdHealerS3(c *C) {
	h := bootstrapInstanceIdHealer{}
	s3 := h.s3()
	c.Assert(s3.Region, DeepEquals, aws.USEast)
}

func (s *S) TestInstanceAgentsConfigHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("instance-agents-config")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, instanceAgentsConfigHealer{})
}

func (s *S) TestInstanceAgenstConfigHealerGetEC2(c *C) {
	h := instanceAgentsConfigHealer{}
	ec2 := h.ec2()
	c.Assert(ec2.EC2Endpoint, Equals, "")
}

func (s *S) TestInstanceAgenstConfigHealerHeal(c *C) {
	jujuTmpdir, err := commandmocker.Add("juju", collectOutputInstanceDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	sshTmpdir, err := commandmocker.Error("ssh", "", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(sshTmpdir)
	h := instanceAgentsConfigHealer{}
	err = h.Heal()
	c.Assert(err, IsNil)
	jujuOutput := []string{
		"status", // for juju status that gets the output
		"status", // for get the bootstrap private dns
	}
	sshOutput := []string{
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"grep",
		"",
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
		"'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\":2181\"/g'",
		"/etc/init/juju-machine-agent.conf",
		"-o",
		"StrictHostKeyChecking no",
		"-q",
		"-l",
		"ubuntu",
		"server-1081.novalocal",
		"grep",
		"",
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
		"'s/env JUJU_ZOOKEEPER=.*/env JUJU_ZOOKEEPER=\":2181\"/g'",
		"/etc/init/juju-as_i_rise-0.conf",
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
	c.Assert(commandmocker.Ran(sshTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(sshTmpdir), DeepEquals, sshOutput)
}

func (s *S) TestBootstrapPrivateDns(c *C) {
	server, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer server.Quit()
	h := instanceAgentsConfigHealer{}
	region := aws.SAEast
	region.EC2Endpoint = server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1})
	c.Assert(err, IsNil)
	instance := resp.Instances[0]
	output := `machines:
  0:
    agent-state: running
    dns-name: localhost
    instance-id: %s
    instance-state: running`
	output = fmt.Sprintf(output, instance.InstanceId)
	jujuTmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(jujuTmpdir)
	dns, err := h.bootstrapPrivateDns()
	c.Assert(err, IsNil)
	c.Assert(dns, Equals, instance.PrivateDNSName)
	jujuOutput := []string{
		"status", // for juju status that gets the output
	}
	c.Assert(commandmocker.Ran(jujuTmpdir), Equals, true)
	c.Assert(commandmocker.Parameters(jujuTmpdir), DeepEquals, jujuOutput)
}

func (s *S) TestGetPrivateDns(c *C) {
	server, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer server.Quit()
	h := instanceAgentsConfigHealer{}
	region := aws.SAEast
	region.EC2Endpoint = server.URL()
	h.e = ec2.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	resp, err := h.ec2().RunInstances(&ec2.RunInstances{MaxCount: 1})
	c.Assert(err, IsNil)
	instance := resp.Instances[0]
	dns, err := h.getPrivateDns(instance.InstanceId)
	c.Assert(err, IsNil)
	c.Assert(dns, Equals, instance.PrivateDNSName)
}

func (s *S) TestInstanceUnitShouldBeRegistered(c *C) {
	h, err := heal.Get("instance-unit")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, instanceUnitHealer{})
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
	c.Assert(h, FitsTypeOf, instanceMachineHealer{})
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
	c.Assert(h, FitsTypeOf, zookeeperHealer{})
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
	h := zookeeperHealer{}
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
	h := zookeeperHealer{}
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
	h := zookeeperHealer{}
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
	h := zookeeperHealer{}
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
	c.Assert(h, FitsTypeOf, bootstrapProvisionHealer{})
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
	c.Assert(h, FitsTypeOf, bootstrapMachineHealer{})
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

func (s *S) TestELBInstanceHealerShouldBeRegistered(c *C) {
	h, err := heal.Get("elb-instance")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, elbInstanceHealer{})
}

func (s *S) TestELBInstanceHealerCheckInstancesDisabledELB(c *C) {
	healer := elbInstanceHealer{}
	instances, err := healer.checkInstances(nil)
	c.Assert(err, IsNil)
	c.Assert(instances, HasLen, 0)
}

func (s *ELBSuite) TestELBInstanceHealerGetUnhealthyApps(c *C) {
	conn, err := db.Conn()
	c.Assert(err, IsNil)
	apps := []interface{}{
		app.App{Name: "when", Units: []app.Unit{{Name: "when/0", State: "started"}}},
		app.App{Name: "what", Units: []app.Unit{{Name: "what/0", State: "error"}}},
		app.App{Name: "why", Units: []app.Unit{{Name: "why/0", State: "down"}}},
		app.App{Name: "how", Units: []app.Unit{
			{Name: "how/0", State: "started"},
			{Name: "how/1", State: "down"},
		}},
	}
	err = conn.Apps().Insert(apps...)
	c.Assert(err, IsNil)
	defer conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{"what", "when", "why", "how"}}})
	healer := elbInstanceHealer{}
	unhealthy := healer.getUnhealthyApps()
	expected := map[string]app.App{
		"what": apps[1].(app.App),
		"why":  apps[2].(app.App),
		"how":  apps[3].(app.App),
	}
	c.Assert(unhealthy, DeepEquals, expected)
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
	healer := elbInstanceHealer{}
	instances, err := healer.checkInstances([]string{lb})
	c.Assert(err, IsNil)
	expected := []elbInstance{
		{
			description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
			state:       "OutOfService",
			reasonCode:  "Instance",
			id:          instance,
			lb:          "elbtest",
		},
	}
	c.Assert(instances, DeepEquals, expected)
}
