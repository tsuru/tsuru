// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"context"
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
	got, err := Get(context.TODO(), "mine")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, got)
	c.Assert(names, check.DeepEquals, []string{"mine"})
	c.Assert(getters, check.HasLen, 1)
	getterType, err := getters[0].GetString("type")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "router")
	_, err = Get(context.TODO(), "unknown-router")
	c.Assert(err, check.DeepEquals, &ErrRouterNotFound{Name: "unknown-router"})
	config.Set("routers:mine-unknown:type", "unknown")
	defer config.Unset("routers:mine-unknown")
	_, err = Get(context.TODO(), "mine-unknown")
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(`unknown router: "unknown".`, check.Equals, err.Error())
}

func (s *S) TestRegisterAndGetWithPlanRouter(c *check.C) {
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
	got, gotPlanRouter, err := GetWithPlanRouter(context.TODO(), "mine")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, got)
	c.Assert(names, check.DeepEquals, []string{"mine"})
	c.Assert(gotPlanRouter, check.DeepEquals, router.PlanRouter{
		Name: "mine",
		Type: "router",
	})
	c.Assert(getters, check.HasLen, 1)
	getterType, err := getters[0].GetString("type")
	c.Assert(err, check.IsNil)
	c.Assert(getterType, check.Equals, "router")
	_, err = Get(context.TODO(), "unknown-router")
	c.Assert(err, check.DeepEquals, &ErrRouterNotFound{Name: "unknown-router"})
	config.Set("routers:mine-unknown:type", "unknown")
	defer config.Unset("routers:mine-unknown")
	_, err = Get(context.TODO(), "mine-unknown")
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
	_, err = Get(context.TODO(), "unknown-router")
	c.Assert(err, check.DeepEquals, &ErrRouterNotFound{Name: "unknown-router"})
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
	_, err := Get(context.TODO(), "inst1")
	c.Assert(err, check.IsNil)
	_, err = Get(context.TODO(), "inst2")
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

	err := servicemanager.DynamicRouter.Create(context.TODO(), router.DynamicRouter{
		Name: "inst2",
		Type: "myrouter",
		Config: map[string]interface{}{
			"cfg1": "v2",
		},
	})
	c.Assert(err, check.IsNil)

	_, planRouter1, err := GetWithPlanRouter(context.TODO(), "inst1")
	c.Assert(err, check.IsNil)
	c.Assert(planRouter1, check.DeepEquals, router.PlanRouter{
		Name:   "inst1",
		Type:   "myrouter",
		Config: map[string]interface{}{"cfg1": "v1"},
	})
	_, planRouter2, err := GetWithPlanRouter(context.TODO(), "inst2")
	c.Assert(err, check.IsNil)
	c.Assert(planRouter2, check.DeepEquals, router.PlanRouter{
		Name:    "inst2",
		Type:    "myrouter",
		Config:  map[string]interface{}{"cfg1": "v2"},
		Dynamic: true,
	})

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
	d, err := Default(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(d, check.Equals, "fake2")
}

func (s *S) TestDefaultNoRouter(c *check.C) {
	d, err := Default(context.TODO())
	c.Assert(err, check.NotNil)
	c.Assert(d, check.Equals, "")
}

func (s *S) TestDefaultNoRouterMultipleRouters(c *check.C) {
	defer config.Unset("routers")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake2:type", "fake")
	d, err := Default(context.TODO())
	c.Assert(err, check.NotNil)
	c.Assert(d, check.Equals, "")
}

func (s *S) TestDefaultSingleRouter(c *check.C) {
	defer config.Unset("routers")
	config.Set("routers:fake:type", "fake")
	d, err := Default(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(d, check.Equals, "fake")
}

func (s *S) TestList(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:default", true)
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	expected := []router.PlanRouter{
		{Name: "router1", Type: "foo", Default: false},
		{Name: "router2", Type: "bar", Default: true},
	}
	routers, err := List(context.TODO())
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

	err := servicemanager.DynamicRouter.Create(context.TODO(), router.DynamicRouter{
		Name: "router-dyn",
		Type: "myrouter",
		Config: map[string]interface{}{
			"mycfg": "zzz",
		},
	})
	c.Assert(err, check.IsNil)

	expected := []router.PlanRouter{
		{Name: "router-dyn", Type: "myrouter", Dynamic: true, Config: map[string]interface{}{"mycfg": "zzz"}},
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar", Config: map[string]interface{}{"cfg1": "aaa"}},
	}
	routers, err := List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

type testInfoRouter struct{ Router }

var _ Router = &testInfoRouter{}

func (r *testInfoRouter) GetInfo(ctx context.Context) (map[string]string, error) {
	return map[string]string{"her": "amaat"}, nil
}

type testInfoErrRouter struct{ Router }

var _ Router = &testInfoErrRouter{}

func (r *testInfoErrRouter) GetInfo(ctx context.Context) (map[string]string, error) {
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
	expected := []router.PlanRouter{
		{Name: "router1", Type: "foo", Info: map[string]string{"her": "amaat"}, Default: false},
		{Name: "router2", Type: "bar", Info: map[string]string{"her": "amaat"}, Default: true},
	}
	routers, err := ListWithInfo(context.TODO())
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
	expected := []router.PlanRouter{
		{Name: "router1", Type: "foo", Info: map[string]string{"her": "amaat"}, Default: false},
		{Name: "router2", Type: "bar", Info: map[string]string{"error": "error getting router info"}, Default: true},
		{Name: "router3", Type: "baz", Info: map[string]string{"error": "create error"}, Default: false},
	}
	routers, err := ListWithInfo(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRouteError(c *check.C) {
	err := &RouterError{Op: "add", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router add] Fatal error.")
	err = &RouterError{Op: "del", Err: errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "[router del] Fatal error.")
}
