// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type mongodbBaseTest struct{}

func (t *mongodbBaseTest) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	config.Set("database:name", "tsuru_storage_mongodb_test")
}

func (t *mongodbBaseTest) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(cacheCollection(conn).Database)
	c.Assert(err, check.IsNil)
}

func (t *mongodbBaseTest) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = cacheCollection(conn).Database.DropDatabase()
	c.Assert(err, check.IsNil)
}

func (t *mongodbBaseTest) TearDownTest(c *check.C) {
}
