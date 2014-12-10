// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
)

func (s *S) TestRegisterAndGet(c *gocheck.C) {
	var r Router
	var prefixes []string
	routerCreator := func(prefix string) (Router, error) {
		prefixes = append(prefixes, prefix)
		return r, nil
	}
	Register("router", routerCreator)
	got, err := Get("router")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.DeepEquals, got)
	c.Assert(prefixes, gocheck.DeepEquals, []string{"router"})
	_, err = Get("unknown-router")
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	expectedMessage := `Unknown router: "unknown-router".`
	c.Assert(expectedMessage, gocheck.Equals, err.Error())
}

func (s *S) TestRegisterAndGetCustomNamedRouter(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	_, err = Get("inst2")
	c.Assert(err, gocheck.IsNil)
	c.Assert(prefixes, gocheck.DeepEquals, []string{"routers:inst1", "routers:inst2"})
}

func (s *S) TestStore(c *gocheck.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, gocheck.IsNil)
	name, err := Retrieve("appname")
	c.Assert(err, gocheck.IsNil)
	c.Assert(name, gocheck.Equals, "routername")
	err = Remove("appname")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRetrieveWithoutKind(c *gocheck.C) {
	err := Store("appname", "routername", "")
	c.Assert(err, gocheck.IsNil)
	data, err := retrieveRouterData("appname")
	c.Assert(err, gocheck.IsNil)
	delete(data, "_id")
	c.Assert(data, gocheck.DeepEquals, map[string]string{
		"app":    "appname",
		"router": "routername",
		"kind":   "hipache",
	})
}

func (s *S) TestRetireveNotFound(c *gocheck.C) {
	name, err := Retrieve("notfound")
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	c.Assert("", gocheck.Equals, name)
}

func (s *S) TestSwapBackendName(c *gocheck.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, gocheck.IsNil)
	defer Remove("appname")
	err = Store("appname2", "routername2", "fake")
	c.Assert(err, gocheck.IsNil)
	defer Remove("appname2")
	err = swapBackendName("appname", "appname2")
	name, err := Retrieve("appname")
	c.Assert(err, gocheck.IsNil)
	c.Assert(name, gocheck.Equals, "routername2")
	name, err = Retrieve("appname2")
	c.Assert(err, gocheck.IsNil)
	c.Assert(name, gocheck.Equals, "routername")
}
