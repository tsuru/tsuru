package main

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/db"
	tec2 "github.com/timeredbull/tsuru/ec2"
	"labix.org/v2/mgo"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session   *mgo.Session
	tmpdir    string
	ec2Srv    *ec2test.Server
	instances []string
	secGroup  ec2.SecurityGroup
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.ec2Srv, err = ec2test.NewServer()
	c.Assert(err, IsNil)
	s.ec2Srv.SetInitialInstanceState(ec2test.Running)
	s.reconfEc2Srv(c)
	s.createTestInstances(c)
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_collector_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	defer s.ec2Srv.Quit()
	_, err := tec2.EC2.TerminateInstances(s.instances)
	if err != nil {
		c.Fail()
	}
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	_, err := db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
	_, err = db.Session.Units().RemoveAll(nil)
	c.Assert(err, IsNil)
}

func (s *S) reconfEc2Srv(c *C) {
	region := aws.Region{EC2Endpoint: s.ec2Srv.URL()}
	auth := aws.Auth{AccessKey: "blaa", SecretKey: "blee"}
	tec2.EC2 = ec2.New(auth, region)
}

func (s *S) createTestInstances(c *C) {
	secGroupResp, err := tec2.EC2.CreateSecurityGroup("default", "default security group")
	s.secGroup = secGroupResp.SecurityGroup
	c.Assert(err, IsNil)
	opts := ec2.RunInstances{
		ImageId:        "ami-0000007",
		SecurityGroups: []ec2.SecurityGroup{s.secGroup},
		MaxCount:       2,
		MinCount:       2,
	}
	instResp, err := tec2.EC2.RunInstances(&opts)
	if err != nil {
		c.Fail()
	}
	s.instances = instancesIds(instResp.Instances)
}
