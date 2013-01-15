// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
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

func getApp(conn *db.Storage, c *C) *app.App {
	a := &app.App{Name: "umaappqq"}
	err := conn.Apps().Insert(&a)
	c.Assert(err, IsNil)
	return a
}

func (s *S) TestUpdate(c *C) {
	a := getApp(s.conn, c)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.Units[0].Name, Equals, "i-00000zz8")
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceId, Equals, "i-0800")
	c.Assert(a.Units[0].State, Equals, provision.StatusStarted.String())
	addr, _ := s.provisioner.Addr(a)
	c.Assert(a.Ip, Equals, addr)
}

func (s *S) TestUpdateWithMultipleUnits(c *C) {
	a := getApp(s.conn, c)
	out := getOutput()
	u := provision.Unit{
		Name:       "i-00000zz9",
		AppName:    "umaappqq",
		Type:       "python",
		Machine:    2,
		InstanceId: "i-0900",
		Ip:         "192.168.0.12",
		Status:     provision.StatusStarted,
	}
	out = append(out, u)
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(len(a.Units), Equals, 2)
	var unit app.Unit
	for _, unit = range a.Units {
		if unit.Machine == 2 {
			break
		}
	}
	c.Assert(unit.Name, Equals, "i-00000zz9")
	c.Assert(unit.Ip, Equals, "192.168.0.12")
	c.Assert(unit.InstanceId, Equals, "i-0900")
	c.Assert(unit.State, Equals, provision.StatusStarted.String())
	addr, _ := s.provisioner.Addr(a)
	c.Assert(a.Ip, Equals, addr)
}

func (s *S) TestUpdateWithDownMachine(c *C) {
	a := app.App{Name: "barduscoapp"}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, IsNil)
	units := []provision.Unit{
		{
			Name:    "i-00000zz8",
			AppName: "barduscoapp",
			Type:    "python",
			Machine: 2,
			Ip:      "",
			Status:  provision.StatusPending,
		},
	}
	update(units)
	err = a.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestUpdateTwice(c *C) {
	a := getApp(s.conn, c)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceId, Equals, "i-0800")
	c.Assert(a.Units[0].State, Equals, provision.StatusStarted.String())
	update(out)
	err = a.Get()
	c.Assert(len(a.Units), Equals, 1)
}

func (s *S) TestUpdateWithMultipleApps(c *C) {
	appDicts := []map[string]string{
		{
			"name": "andrewzito3",
			"ip":   "10.10.10.163",
		},
		{
			"name": "flaviapp",
			"ip":   "10.10.10.208",
		},
		{
			"name": "mysqlapi",
			"ip":   "10.10.10.131",
		},
		{
			"name": "teste_api_semantica",
			"ip":   "10.10.10.189",
		},
		{
			"name": "xikin",
			"ip":   "10.10.10.168",
		},
	}
	apps := make([]app.App, len(appDicts))
	units := make([]provision.Unit, len(appDicts))
	for i, appDict := range appDicts {
		a := app.App{Name: appDict["name"]}
		err := s.conn.Apps().Insert(&a)
		c.Assert(err, IsNil)
		apps[i] = a
		units[i] = provision.Unit{
			Name:    "i-00000",
			AppName: appDict["name"],
			Machine: i + 1,
			Type:    "python",
			Ip:      appDict["ip"],
			Status:  provision.StatusInstalling,
		}
	}
	update(units)
	for _, appDict := range appDicts {
		a := app.App{Name: appDict["name"]}
		err := a.Get()
		c.Assert(err, IsNil)
		c.Assert(a.Units[0].Ip, Equals, appDict["ip"])
	}
}
