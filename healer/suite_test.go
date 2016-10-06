// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(&S{})

type S struct{}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "healer_tests")
	config.Set("docker:repository-namespace", "tsuru")
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
}

func (s *S) SetUpTest(c *check.C) {
	HealerInstance = nil
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	iaas.ResetAll()
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}
