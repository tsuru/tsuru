package main

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
	tmpdir  string
	ec2Srv  *ec2test.Server
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.ec2Srv, err = ec2test.NewServer()
	c.Assert(err, IsNil)
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_collector_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	s.ec2Srv.Quit()
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
	tsuruEC2.EC2 = ec2.New(auth, region)
}
