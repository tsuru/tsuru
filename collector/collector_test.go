// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/provision"
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
	c.Assert(a.Units[0].Name, gocheck.Equals, "i-00000zz8")
	c.Assert(a.Units[0].Ip, gocheck.Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, gocheck.Equals, 1)
	c.Assert(a.Units[0].InstanceId, gocheck.Equals, "i-0800")
	c.Assert(a.Units[0].State, gocheck.Equals, provision.StatusStarted.String())
	addr, _ := s.provisioner.Addr(a)
	c.Assert(a.Ip, gocheck.Equals, addr)
}

func (s *S) TestUpdateWithMultipleUnits(c *gocheck.C) {
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
	a, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(a.Units), gocheck.Equals, 2)
	var unit app.Unit
	for _, unit = range a.Units {
		if unit.Machine == 2 {
			break
		}
	}
	c.Assert(unit.Name, gocheck.Equals, "i-00000zz9")
	c.Assert(unit.Ip, gocheck.Equals, "192.168.0.12")
	c.Assert(unit.InstanceId, gocheck.Equals, "i-0900")
	c.Assert(unit.State, gocheck.Equals, provision.StatusStarted.String())
	addr, _ := s.provisioner.Addr(a)
	c.Assert(a.Ip, gocheck.Equals, addr)
	c.Assert(a.State, gocheck.Equals, "ready")
}

func (s *S) TestUpdateWithDownMachine(c *gocheck.C) {
	a := app.App{Name: "barduscoapp"}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	units := []provision.Unit{
		{
			Name:    "i-00000zz8",
			AppName: "barduscoapp",
			Type:    "python",
			Machine: 2,
			Ip:      "",
			Status:  provision.StatusBuilding,
		},
	}
	update(units)
	_, err = app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUpdateTwice(c *gocheck.C) {
	a := getApp(s.conn, c)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	out := getOutput()
	update(out)
	a, err := app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Units[0].Ip, gocheck.Equals, "192.168.0.11")
	c.Assert(a.Units[0].Machine, gocheck.Equals, 1)
	c.Assert(a.Units[0].InstanceId, gocheck.Equals, "i-0800")
	c.Assert(a.Units[0].State, gocheck.Equals, provision.StatusStarted.String())
	update(out)
	a, err = app.GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(a.Units), gocheck.Equals, 1)
}

func (s *S) TestUpdateWithMultipleApps(c *gocheck.C) {
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
		c.Assert(err, gocheck.IsNil)
		apps[i] = a
		units[i] = provision.Unit{
			Name:    "i-00000",
			AppName: appDict["name"],
			Machine: i + 1,
			Type:    "python",
			Ip:      appDict["ip"],
			Status:  provision.StatusBuilding,
		}
	}
	update(units)
	for _, appDict := range appDicts {
		a, err := app.GetByName(appDict["name"])
		c.Assert(err, gocheck.IsNil)
		c.Assert(a.Units[0].Ip, gocheck.Equals, appDict["ip"])
	}
}
