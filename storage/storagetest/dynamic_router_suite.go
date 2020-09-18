// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

type DynamicRouterSuite struct {
	SuiteHooks
	DynamicRouterStorage router.DynamicRouterStorage
}

func (s *DynamicRouterSuite) TestSave(c *check.C) {
	dr := router.DynamicRouter{
		Name: "my-router",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err := s.DynamicRouterStorage.Save(context.TODO(), dr)
	c.Assert(err, check.IsNil)
	rtDB, err := s.DynamicRouterStorage.Get(context.TODO(), "my-router")
	c.Assert(err, check.IsNil)
	c.Assert(rtDB, check.DeepEquals, &dr)
}

func (s *DynamicRouterSuite) TestGetNotFound(c *check.C) {
	_, err := s.DynamicRouterStorage.Get(context.TODO(), "my-router")
	c.Assert(err, check.Equals, router.ErrDynamicRouterNotFound)
}

func (s *DynamicRouterSuite) TestList(c *check.C) {
	dbs, err := s.DynamicRouterStorage.List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.HasLen, 0)

	dr1 := router.DynamicRouter{
		Name: "my-router1",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	dr2 := router.DynamicRouter{
		Name: "my-router2",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err = s.DynamicRouterStorage.Save(context.TODO(), dr1)
	c.Assert(err, check.IsNil)
	err = s.DynamicRouterStorage.Save(context.TODO(), dr2)
	c.Assert(err, check.IsNil)
	dbs, err = s.DynamicRouterStorage.List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.DeepEquals, []router.DynamicRouter{dr1, dr2})
}

func (s *DynamicRouterSuite) TestR(c *check.C) {
	dr1 := router.DynamicRouter{
		Name: "my-router1",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	dr2 := router.DynamicRouter{
		Name: "my-router2",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err := s.DynamicRouterStorage.Save(context.TODO(), dr1)
	c.Assert(err, check.IsNil)
	err = s.DynamicRouterStorage.Save(context.TODO(), dr2)
	c.Assert(err, check.IsNil)
	err = s.DynamicRouterStorage.Remove(context.TODO(), dr1.Name)
	c.Assert(err, check.IsNil)
	dbs, err := s.DynamicRouterStorage.List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.DeepEquals, []router.DynamicRouter{dr2})
}

func (s *DynamicRouterSuite) TestRemoveNotFound(c *check.C) {
	err := s.DynamicRouterStorage.Remove(context.TODO(), "my-router")
	c.Assert(err, check.Equals, router.ErrDynamicRouterNotFound)
}
