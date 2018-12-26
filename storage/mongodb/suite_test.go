// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type mongodbBaseTest struct {
	name string
}

func (t *mongodbBaseTest) dbName() string {
	n := "tsuru_storage_mongodb_test"
	if t.name != "" {
		n += "_" + t.name
	}
	return n
}

func (t *mongodbBaseTest) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	// Drop database does not force the deletion of the indexes.
	// Because of that, a new collection may have wrong indexes, as it doesn't create new ones and doesn't use the old ones.
	// Therefore, tests may not work properly and you need an individual db for this test suite.
	config.Set("database:name", t.dbName())
}

func (t *mongodbBaseTest) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Storage.Database(t.dbName()))
	c.Assert(err, check.IsNil)
}

func (t *mongodbBaseTest) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Storage.DropDatabase(t.dbName())
	c.Assert(err, check.IsNil)
}

func (t *mongodbBaseTest) TearDownTest(c *check.C) {
}
