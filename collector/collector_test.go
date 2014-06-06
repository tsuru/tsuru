// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

func getOutput() []provision.Unit {
	return []provision.Unit{
		{
			Name:       "i-00000zz8",
			AppName:    "umaappqq",
			Type:       "python",
			Machine:    1,
			InstanceId: "i-0800",
			Ip:         "192.168.0.11",
			Status:     provision.StatusStarted,
		},
	}
}

func getApp(conn *db.Storage, c *gocheck.C) *app.App {
	a := &app.App{Name: "umaappqq"}
	err := conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	return a
}

func (s *S) TestUpdate(c *gocheck.C) {
	a := getApp(s.conn, c)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	a, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	addr, _ := s.provisioner.Addr(a)
	c.Assert(a.Ip, gocheck.Equals, addr)
}

func (s *S) TestUpdateTwice(c *gocheck.C) {
	a := getApp(s.conn, c)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	a, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	update(out)
	a, err = app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
}
