// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"errors"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestRegisterAndGet(c *check.C) {
	var r Router
	var prefixes []string
	var names []string
	routerCreator := func(name, prefix string) (Router, error) {
		names = append(names, name)
		prefixes = append(prefixes, prefix)
		return r, nil
	}
	Register("router", routerCreator)
	config.Set("routers:mine:type", "router")
	defer config.Unset("routers:mine:type")
	got, err := Get("mine")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, got)
	c.Assert(names, check.DeepEquals, []string{"mine"})
	c.Assert(prefixes, check.DeepEquals, []string{"routers:mine"})
	_, err = Get("unknown-router")
	c.Assert(err, check.Not(check.IsNil))
	c.Assert("config key 'routers:unknown-router:type' not found", check.Equals, err.Error())
	config.Set("routers:mine-unknown:type", "unknown")
	defer config.Unset("routers:mine-unknown:type")
	_, err = Get("mine-unknown")
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(`unknown router: "unknown".`, check.Equals, err.Error())
}

func (s *S) TestRegisterAndType(c *check.C) {
	config.Set("routers:mine:type", "myrouter")
	defer config.Unset("routers:mine:type")
	rType, prefix, err := Type("mine")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "myrouter")
	c.Assert(prefix, check.Equals, "routers:mine")
	_, err = Get("unknown-router")
	c.Assert(err, check.ErrorMatches, `config key 'routers:unknown-router:type' not found`)
}

func (s *S) TestRegisterAndTypeSpecialCase(c *check.C) {
	rType, prefix, err := Type("hipache")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "hipache")
	c.Assert(prefix, check.Equals, "hipache")
	config.Set("routers:hipache:type", "htype")
	defer config.Unset("routers:hipache:type")
	rType, prefix, err = Type("hipache")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "htype")
	c.Assert(prefix, check.Equals, "routers:hipache")
}

func (s *S) TestRegisterAndGetCustomNamedRouter(c *check.C) {
	var names []string
	var prefixes []string
	routerCreator := func(name, prefix string) (Router, error) {
		names = append(names, name)
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
	c.Assert(names, check.DeepEquals, []string{"inst1", "inst2"})
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
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	expected := []PlanRouter{
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestListIncludesLegacyHipacheRouter(c *check.C) {
	config.Set("hipache:something", "somewhere")
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("hipache")
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	expected := []PlanRouter{
		{Name: "hipache", Type: "hipache"},
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestListIncludesOnlyLegacyHipacheRouter(c *check.C) {
	config.Set("hipache:something", "somewhere")
	defer config.Unset("hipache")
	expected := []PlanRouter{
		{Name: "hipache", Type: "hipache"},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRouteError(c *check.C) {
	err := &RouterError{Op: "add", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router add] Fatal error.")
	err = &RouterError{Op: "del", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router del] Fatal error.")
}
