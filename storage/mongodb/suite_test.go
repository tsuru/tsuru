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

type S struct {
	conn            *db.Storage
	TeamService     *TeamService
	PlatformService *PlatformService
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_storage_test")
	s.conn, _ = db.Conn()
	s.TeamService = &TeamService{}
	s.PlatformService = &PlatformService{}
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Users().Database.DropDatabase()
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
}
