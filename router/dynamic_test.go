// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func (s *S) TestDynamicRouterServiceCreate(c *check.C) {
	svc, err := DynamicRouterService()
	c.Assert(err, check.IsNil)
	err = svc.Create(router.DynamicRouter{})
	c.Assert(err, check.ErrorMatches, `dynamic router name and type are required`)
	err = svc.Create(router.DynamicRouter{
		Name: "myrouter",
		Type: "invalid",
	})
	c.Assert(err, check.ErrorMatches, `router type "invalid" is not registered`)
	config.Set("routers:mine:type", "myrouter")
	defer config.Unset("routers:mine")
	Register("myrouter", func(name string, config router.ConfigGetter) (Router, error) {
		return nil, nil
	})
	err = svc.Create(router.DynamicRouter{
		Name: "mine",
		Type: "myrouter",
	})
	c.Assert(err, check.ErrorMatches, `router named "mine" already exists in config`)
	err = svc.Create(router.DynamicRouter{
		Name: "mine2",
		Type: "myrouter",
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestDynamicRouterServiceUpdateNotFound(c *check.C) {
	svc, err := DynamicRouterService()
	c.Assert(err, check.IsNil)
	err = svc.Update(router.DynamicRouter{
		Name: "mine",
	})
	c.Assert(err, check.Equals, router.ErrDynamicRouterNotFound)
}

func (s *S) TestDynamicRouterServiceUpdate(c *check.C) {
	svc, err := DynamicRouterService()
	c.Assert(err, check.IsNil)
	Register("myrouter", func(name string, config router.ConfigGetter) (Router, error) {
		return nil, nil
	})
	err = svc.Create(router.DynamicRouter{
		Name: "mine",
		Type: "myrouter",
		Config: map[string]interface{}{
			"a": "b",
			"c": "d",
			"e": "f",
		},
	})
	c.Assert(err, check.IsNil)
	err = svc.Update(router.DynamicRouter{
		Name: "mine",
		Config: map[string]interface{}{
			"a": nil,
			"c": "999",
			"z": "x",
		},
	})
	c.Assert(err, check.IsNil)
	dbDR, err := svc.Get("mine")
	c.Assert(err, check.IsNil)
	c.Assert(dbDR, check.DeepEquals, &router.DynamicRouter{
		Name: "mine",
		Type: "myrouter",
		Config: map[string]interface{}{
			"c": "999",
			"z": "x",
			"e": "f",
		},
	})
}
