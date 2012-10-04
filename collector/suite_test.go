package main

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session   *mgo.Session
	tmpdir    string
	instances []string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_collector_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	_, err := db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
}
