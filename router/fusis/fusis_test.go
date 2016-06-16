// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fusis

import (
	"testing"

	fusisTesting "github.com/luizbafilho/fusis/api/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func init() {
	suite := &routertest.RouterSuite{
		SetUpSuiteFunc: func(c *check.C) {
			config.Set("routers:fusis:domain", "fusis.com")
			config.Set("routers:fusis:type", "fusis")
			config.Set("database:url", "127.0.0.1:27017")
			config.Set("database:name", "router_fusis_tests")
		},
	}
	var fakeServer *fusisTesting.FakeFusisServer
	suite.SetUpTestFunc = func(c *check.C) {
		var err error
		fakeServer = fusisTesting.NewFakeFusisServer()
		config.Set("routers:fusis:api-url", fakeServer.URL)
		fRouter, err := createRouter("fusis", "routers:fusis")
		c.Assert(err, check.IsNil)
		suite.Router = fRouter
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		dbtest.ClearAllCollections(conn.Collection("router_fusis_tests").Database)
	}
	suite.TearDownTestFunc = func(c *check.C) {
		fakeServer.Close()
	}
	check.Suite(suite)
}
