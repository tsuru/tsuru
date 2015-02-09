// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	ticker.Stop()
}

func (s *S) TearDownSuite(c *check.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	defer storage.session.Close()
	storage.session.DB("tsuru_storage_test").DropDatabase()
}

func (s *S) TearDownTest(c *check.C) {
	conn = make(map[string]*session)
}

func (s *S) TestOpenConnectsToTheDatabase(c *check.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	defer storage.session.Close()
	c.Assert(storage.session.Ping(), check.IsNil)
}

func (s *S) TestOpenCopiesConnection(c *check.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	defer storage.session.Close()
	storage2, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	c.Assert(storage.session, check.Not(check.Equals), storage2.session)
}

func (s *S) TestOpenReconnects(c *check.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	storage.session.Close()
	storage, err = Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	err = storage.session.Ping()
	c.Assert(err, check.IsNil)
}

func (s *S) TestOpenConnectionRefused(c *check.C) {
	storage, err := Open("127.0.0.1:27018", "tsuru_storage_test")
	c.Assert(storage, check.IsNil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestClose(c *check.C) {
	defer func() {
		r := recover()
		c.Check(r, check.NotNil)
	}()
	storage, err := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(err, check.IsNil)
	storage.Close()
	err = storage.session.Ping()
	c.Check(err, check.NotNil)
}

func (s *S) TestCollection(c *check.C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.session.Close()
	collection := storage.Collection("users")
	c.Assert(collection.FullName, check.Equals, storage.dbname+".users")
}
