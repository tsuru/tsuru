// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_rebuild_tests")
	config.Set("queue:mongo-url", "127.0.0.1:27017")
	config.Set("queue:mongo-database", "queue_router_rebuild_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake-hc:type", "fake-hc")
	config.Set("docker:router", "fake")
	provision.DefaultProvisioner = "fake"
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	queue.ResetQueue()
	err := rebuild.RegisterTask(func(appName string) (rebuild.RebuildApp, error) {
		a, err := app.GetByName(appName)
		if err == app.ErrAppNotFound {
			return nil, nil
		}
		return a, err
	})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.Reset()
	provisiontest.ProvisionerInstance.Reset()
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
}
