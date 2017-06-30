// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "gandalf_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.User().Database.DropDatabase()
}

func (s *S) TestSessionRepositoryShouldReturnAMongoCollection(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	rep := conn.Repository()
	cRep := conn.Collection("repository")
	c.Assert(rep, check.DeepEquals, cRep)
}

func (s *S) TestSessionUserShouldReturnAMongoCollection(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	usr := conn.User()
	cUsr := conn.Collection("user")
	c.Assert(usr, check.DeepEquals, cUsr)
}

func (s *S) TestSessionKeyShouldReturnKeyCollection(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	key := conn.Key()
	cKey := conn.Collection("key")
	c.Assert(key, check.DeepEquals, cKey)
}

func (s *S) TestSessionKeyIndexes(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	key := conn.Key()
	indexes, err := key.Indexes()
	c.Assert(err, check.IsNil)
	c.Check(indexes, check.HasLen, 3)
	c.Check(indexes[1].Key, check.DeepEquals, []string{"body"})
	c.Check(indexes[1].Unique, check.DeepEquals, true)
	c.Check(indexes[2].Key, check.DeepEquals, []string{"username", "name"})
	c.Check(indexes[2].Unique, check.DeepEquals, true)
}

func (s *S) TestConnect(c *check.C) {
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	c.Assert(conn.User().Database.Name, check.Equals, "gandalf_tests")
	err = conn.User().Database.Session.Ping()
	c.Assert(err, check.IsNil)
}

func (s *S) TestConnectDefaultSettings(c *check.C) {
	oldURL, _ := config.Get("database:url")
	defer config.Set("database:url", oldURL)
	oldName, _ := config.Get("database:name")
	defer config.Set("database:name", oldName)
	config.Unset("database:url")
	config.Unset("database:name")
	conn, err := Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	c.Assert(conn.User().Database.Name, check.Equals, "gandalf")
	c.Assert(conn.User().Database.Session.LiveServers(), check.DeepEquals, []string{"127.0.0.1:27017"})
}

func (s *S) TestDbConfig(c *check.C) {
	oldURL, _ := config.Get("database:url")
	defer config.Set("database:url", oldURL)
	oldName, _ := config.Get("database:name")
	defer config.Set("database:name", oldName)
	url, dbname := DbConfig()
	c.Assert(url, check.Equals, oldURL)
	c.Assert(dbname, check.Equals, oldName)
	config.Unset("database:url")
	config.Unset("database:name")
	url, dbname = DbConfig()
	c.Assert(url, check.Equals, "127.0.0.1:27017")
	c.Assert(dbname, check.Equals, "gandalf")
}
