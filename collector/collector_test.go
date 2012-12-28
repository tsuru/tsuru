// Copyright 2012 tsuru authors. All rights reserved.
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

func getApp(c *C) *app.App {
	a := &app.App{Name: "umaappqq", State: "STOPPED"}
	err := db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	return a
}

func (s *S) TestUpdate(c *C) {
	a := getApp(c)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "started")
	c.Assert(a.Units[0].Name, Equals, "i-00000zz8")
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceId, Equals, "i-0800")
	c.Assert(a.Units[0].State, Equals, string(provision.StatusStarted))
}

func (s *S) TestUpdateWithMultipleUnits(c *C) {
	a := getApp(c)
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
	c.Assert(unit.State, Equals, string(provision.StatusStarted))
}

func (s *S) TestUpdateWithDownMachine(c *C) {
	a := app.App{Name: "barduscoapp"}
	err := db.Session.Apps().Insert(&a)
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
	c.Assert(a.State, Equals, string(provision.StatusPending))
}

func (s *S) TestUpdateTwice(c *C) {
	a := getApp(c)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	err := a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, Equals, "started")
	c.Assert(a.Units[0].Ip, Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, Equals, 1)
	c.Assert(a.Units[0].InstanceId, Equals, "i-0800")
	c.Assert(a.Units[0].State, Equals, string(provision.StatusStarted))
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
		err := db.Session.Apps().Insert(&a)
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
