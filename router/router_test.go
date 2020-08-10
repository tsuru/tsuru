// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func (s *S) TestRegisterAndGet(c *check.C) {
	var r Router
	var getters []ConfigGetter
	var names []string
	routerCreator := func(name string, config ConfigGetter) (Router, error) {
		names = append(names, name)
		getters = append(getters, config)
		return r, nil
	}
	Register("router", routerCreator)
	config.Set("routers:mine:type", "router")
	defer config.Unset("routers:mine")
	got, err := Get("mine")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, got)
	c.Assert(names, check.DeepEquals, []string{"mine"})
	c.Assert(getters, check.HasLen, 1)
	getterType, err := getters[0].GetString("type")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "router")
	_, err = Get("unknown-router")
	c.Assert(err, check.DeepEquals, &ErrRouterNotFound{Name: "unknown-router"})
	config.Set("routers:mine-unknown:type", "unknown")
	defer config.Unset("routers:mine-unknown")
	_, err = Get("mine-unknown")
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(`unknown router: "unknown".`, check.Equals, err.Error())
}

func (s *S) TestRegisterAndType(c *check.C) {
	config.Set("routers:mine:type", "myrouter")
	defer config.Unset("routers:mine")
	rType, prefix, err := configType("mine")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "myrouter")
	c.Assert(prefix, check.Equals, "routers:mine")
	_, err = Get("unknown-router")
	c.Assert(err, check.DeepEquals, &ErrRouterNotFound{Name: "unknown-router"})
}

func (s *S) TestRegisterAndTypeSpecialCase(c *check.C) {
	rType, prefix, err := configType("hipache")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "hipache")
	c.Assert(prefix, check.Equals, "hipache")
	config.Set("routers:hipache:type", "htype")
	defer config.Unset("routers:hipache:type")
	rType, prefix, err = configType("hipache")
	c.Assert(err, check.IsNil)
	c.Assert(rType, check.Equals, "htype")
	c.Assert(prefix, check.Equals, "routers:hipache")
}

func (s *S) TestRegisterAndGetCustomNamedRouter(c *check.C) {
	var names []string
	var getters []ConfigGetter
	routerCreator := func(name string, config ConfigGetter) (Router, error) {
		names = append(names, name)
		getters = append(getters, config)
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
	c.Assert(getters, check.HasLen, 2)
	getterType, err := getters[0].GetString("type")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "myrouter")
	getterType, err = getters[1].GetString("type")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "myrouter")
}

func (s *S) TestGetDynamicRouter(c *check.C) {
	var names []string
	var getters []ConfigGetter
	routerCreator := func(name string, config ConfigGetter) (Router, error) {
		names = append(names, name)
		getters = append(getters, config)
		var r Router
		return r, nil
	}
	Register("myrouter", routerCreator)
	config.Set("routers:inst1:type", "myrouter")
	config.Set("routers:inst1:cfg1", "v1")
	defer config.Unset("routers:inst1")

	err := servicemanager.DynamicRouter.Create(router.DynamicRouter{
		Name: "inst2",
		Type: "myrouter",
		Config: map[string]interface{}{
			"cfg1": "v2",
		},
	})
	c.Assert(err, check.IsNil)

	_, err = Get("inst1")
	c.Assert(err, check.IsNil)
	_, err = Get("inst2")
	c.Assert(err, check.IsNil)

	c.Assert(names, check.DeepEquals, []string{"inst1", "inst2"})
	c.Assert(getters, check.HasLen, 2)
	getterType, err := getters[0].GetString("cfg1")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "v1")
	getterType, err = getters[1].GetString("cfg1")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "v2")
}

func (s *S) TestDefault(c *check.C) {
	defer config.Unset("routers")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake2:type", "fake")
	config.Set("routers:fake2:default", true)
	d, err := Default()
	c.Assert(err, check.IsNil)
	c.Assert(d, check.Equals, "fake2")
}

func (s *S) TestDefaultNoRouter(c *check.C) {
	d, err := Default()
	c.Assert(err, check.NotNil)
	c.Assert(d, check.Equals, "")
}

func (s *S) TestDefaultNoRouterMultipleRouters(c *check.C) {
	defer config.Unset("routers")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake2:type", "fake")
	d, err := Default()
	c.Assert(err, check.NotNil)
	c.Assert(d, check.Equals, "")
}

func (s *S) TestDefaultSingleRouter(c *check.C) {
	defer config.Unset("routers")
	config.Set("routers:fake:type", "fake")
	d, err := Default()
	c.Assert(err, check.IsNil)
	c.Assert(d, check.Equals, "fake")
}

func (s *S) TestDefaultFromFallbackConfig(c *check.C) {
	defer config.Unset("routers")
	defer config.Unset("docker:router")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake2:type", "fake")
	config.Set("docker:router", "fake2")
	d, err := Default()
	c.Assert(err, check.IsNil)
	c.Assert(d, check.Equals, "fake2")
}

func (s *S) TestStore(c *check.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, check.IsNil)
	name, err := Retrieve("appname")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "routername")
	err = Remove("appname")
	c.Assert(err, check.IsNil)
	_, err = Retrieve("appname")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestStoreUpdatesEntry(c *check.C) {
	err := Store("appname", "routername", "fake")
	c.Assert(err, check.IsNil)
	err = Store("appname", "routername2", "fake2")
	c.Assert(err, check.IsNil)
	name, err := Retrieve("appname")
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, "routername2")
	data, err := retrieveRouterData("appname")
	c.Assert(err, check.IsNil)
	data.ID = ""
	c.Assert(data, check.DeepEquals, routerAppEntry{
		App:    "appname",
		Router: "routername2",
		Kind:   "fake2",
	})
	err = Remove("appname")
	c.Assert(err, check.IsNil)
	_, err = Retrieve("appname")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestRetrieveWithoutKind(c *check.C) {
	err := Store("appname", "routername", "")
	c.Assert(err, check.IsNil)
	data, err := retrieveRouterData("appname")
	c.Assert(err, check.IsNil)
	data.ID = ""
	c.Assert(data, check.DeepEquals, routerAppEntry{
		App:    "appname",
		Router: "routername",
		Kind:   "hipache",
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
	c.Assert(err, check.IsNil)
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
	config.Set("routers:router2:default", true)
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	expected := []PlanRouter{
		{Name: "router1", Type: "foo", Default: false},
		{Name: "router2", Type: "bar", Default: true},
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

func (s *S) TestListDefaultDockerRouter(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("docker:router", "router2")
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	defer config.Unset("docker:router")
	expected := []PlanRouter{
		{Name: "router1", Type: "foo", Default: false},
		{Name: "router2", Type: "bar", Default: true},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestListIncludesDynamic(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:cfg1", "aaa")
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")

	Register("myrouter", func(name string, config ConfigGetter) (Router, error) {
		return nil, nil
	})

	err := servicemanager.DynamicRouter.Create(router.DynamicRouter{
		Name: "router-dyn",
		Type: "myrouter",
		Config: map[string]interface{}{
			"mycfg": "zzz",
		},
	})
	c.Assert(err, check.IsNil)

	expected := []PlanRouter{
		{Name: "router-dyn", Type: "myrouter", Dynamic: true, Config: map[string]interface{}{"mycfg": "zzz"}},
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar", Config: map[string]interface{}{"cfg1": "aaa"}},
	}
	routers, err := List()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

type testInfoRouter struct{ Router }

func (r *testInfoRouter) GetInfo() (map[string]string, error) {
	return map[string]string{"her": "amaat"}, nil
}

type testInfoErrRouter struct{ Router }

func (r *testInfoErrRouter) GetInfo() (map[string]string, error) {
	return nil, errors.New("error getting router info")
}

func (s *S) TestListWithInfo(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:default", true)
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	fooCreator := func(name string, config ConfigGetter) (Router, error) {
		return &testInfoRouter{}, nil
	}
	Register("foo", fooCreator)
	Register("bar", fooCreator)
	expected := []PlanRouter{
		{Name: "router1", Type: "foo", Info: map[string]string{"her": "amaat"}, Default: false},
		{Name: "router2", Type: "bar", Info: map[string]string{"her": "amaat"}, Default: true},
	}
	routers, err := ListWithInfo()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestListWithInfoError(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:default", true)
	config.Set("routers:router3:type", "baz")
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	defer config.Unset("routers:router3")
	fooCreator := func(name string, config ConfigGetter) (Router, error) {
		return &testInfoRouter{}, nil
	}
	barCreator := func(name string, config ConfigGetter) (Router, error) {
		return &testInfoErrRouter{}, nil
	}
	bazCreator := func(name string, config ConfigGetter) (Router, error) {
		return nil, errors.New("create error")
	}
	Register("foo", fooCreator)
	Register("bar", barCreator)
	Register("baz", bazCreator)
	expected := []PlanRouter{
		{Name: "router1", Type: "foo", Info: map[string]string{"her": "amaat"}, Default: false},
		{Name: "router2", Type: "bar", Info: map[string]string{"error": "error getting router info"}, Default: true},
		{Name: "router3", Type: "baz", Info: map[string]string{"error": "create error"}, Default: false},
	}
	routers, err := ListWithInfo()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRouteError(c *check.C) {
	err := &RouterError{Op: "add", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router add] Fatal error.")
	err = &RouterError{Op: "del", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router del] Fatal error.")
}
