// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type S struct {
	conn *db.Storage
}

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

var _ = check.Suite(&S{})

func (s *S) TestMigrateAppPlanRouterToRouter(c *check.C) {
	config.Set("routers:galeb:default", true)
	defer config.Unset("routers")
	a := &app.App{Name: "with-plan-router"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Update(bson.M{"name": "with-plan-router"}, bson.M{"$set": bson.M{"plan.router": "planb"}})
	c.Assert(err, check.IsNil)
	a = &app.App{Name: "without-plan-router"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	a = &app.App{Name: "with-router", Router: "hipache"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = MigrateAppPlanRouterToRouter()
	c.Assert(err, check.IsNil)
	a, err = app.GetByName("with-plan-router")
	c.Assert(err, check.IsNil)
	c.Assert(a.Router, check.Equals, "planb")
	a, err = app.GetByName("without-plan-router")
	c.Assert(err, check.IsNil)
	c.Assert(a.Router, check.Equals, "galeb")
	a, err = app.GetByName("with-router")
	c.Assert(err, check.IsNil)
	c.Assert(a.Router, check.Equals, "hipache")
}

func (s *S) TestMigrateAppPlanRouterToRouterWithoutDefaultRouter(c *check.C) {
	err := MigrateAppPlanRouterToRouter()
	c.Assert(err, check.DeepEquals, router.ErrDefaultRouterNotFound)
}
