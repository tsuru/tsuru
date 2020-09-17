// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "api_tracker_pkg_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) Test_InstanceService(c *check.C) {
	svc, err := InstanceService()
	c.Assert(err, check.IsNil)
	svc.(*instanceTracker).Shutdown(context.Background())
	instances, err := svc.LiveInstances()
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
	c.Assert(instances[0].Name, check.Not(check.Equals), "")
	c.Assert(len(instances[0].Addresses) > 0, check.Equals, true)
}

func (s *S) Test_InstanceService_CurrentInstance(c *check.C) {
	svc, err := InstanceService()
	c.Assert(err, check.IsNil)
	svc.(*instanceTracker).Shutdown(context.Background())
	instance, err := svc.CurrentInstance()
	c.Assert(err, check.IsNil)
	c.Assert(instance.Name, check.Not(check.Equals), "")
	c.Assert(len(instance.Addresses) > 0, check.Equals, true)
}
