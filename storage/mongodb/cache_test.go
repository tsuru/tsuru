// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/storage/storagetest"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.CacheSuite{
	CacheService: &cacheService{},
	CustomSuite: storagetest.CustomSuite{
		SetUpSuiteFn: func(c *check.C) {
			config.Set("database:url", "127.0.0.1:27017")
			config.Set("database:name", "tsuru_storage_mongodb_cache_test")
		},
		SetUpTestFn: func(c *check.C) {
			conn, err := db.Conn()
			c.Assert(err, check.IsNil)
			err = dbtest.ClearAllCollections(cacheCollection(conn).Database)
			c.Assert(err, check.IsNil)
		},
		TearDownSuiteFn: func(c *check.C) {
			conn, err := db.Conn()
			c.Assert(err, check.IsNil)
			err = cacheCollection(conn).Database.DropDatabase()
			c.Assert(err, check.IsNil)
		},
	},
})
