// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestRegisterAndGet(c *check.C) {
	var r Router
	var prefixes []string
	routerCreator := func(prefix string) (Router, error) {
		prefixes = append(prefixes, prefix)
		return r, nil
	}
	Register("router", routerCreator)
	got, err := Get("router")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, got)
	c.Assert(prefixes, check.DeepEquals, []string{"router"})
	_, err = Get("unknown-router")
	c.Assert(err, check.Not(check.IsNil))
	expectedMessage := `Unknown router: "unknown-router".`
	c.Assert(expectedMessage, check.Equals, err.Error())
}

func (s *S) TestRegisterAndGetCustomNamedRouter(c *check.C) {
	var prefixes []string
	routerCreator := func(prefix string) (Router, error) {
		prefixes = append(prefixes, prefix)
		var r Router
		return r, nil
	}
	Register("myrouter", routerCreator)
	config.Set("routers:inst1:type", "myrouter")
	config.Set("routers:inst2:type", "myrouter")
	defer config.Unset("routers:inst1:type")
	defer config.Unset("routers:inst2:type")
	_, err := Get("inst1")
	c.Assert(err, check.IsNil)
	_, err = Get("inst2")
	c.Assert(err, check.IsNil)
	c.Assert(prefixes, check.DeepEquals, []string{"routers:inst1", "routers:inst2"})
}

func (s *S) TestStore(c *check.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, check.IsNil)
	name, err := Retrieve("appname")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "routername")
	err = Remove("appname")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRetrieveWithoutKind(c *check.C) {
	err := Store("appname", "routername", "")
	c.Assert(err, check.IsNil)
	data, err := retrieveRouterData("appname")
	c.Assert(err, check.IsNil)
	delete(data, "_id")
	c.Assert(data, check.DeepEquals, map[string]string{
		"app":    "appname",
		"router": "routername",
		"kind":   "hipache",
	})
}

func (s *S) TestRetireveNotFound(c *check.C) {
	name, err := Retrieve("notfound")
	c.Assert(err, check.Not(check.IsNil))
	c.Assert("", check.Equals, name)
}

func (s *S) TestSwapBackendName(c *check.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, check.IsNil)
	defer Remove("appname")
	err = Store("appname2", "routername2", "fake")
	c.Assert(err, check.IsNil)
	defer Remove("appname2")
	err = swapBackendName("appname", "appname2")
	name, err := Retrieve("appname")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "routername2")
	name, err = Retrieve("appname2")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "routername")
}

func (s *S) TestList(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	expected := []PlanRouter{
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}
