// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

type RouterTemplateSuite struct {
	SuiteHooks
	RouterTemplateStorage router.RouterTemplateStorage
}

func (s *RouterTemplateSuite) TestSave(c *check.C) {
	rt := router.RouterTemplate{
		Name: "my-router",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err := s.RouterTemplateStorage.Save(rt)
	c.Assert(err, check.IsNil)
	rtDB, err := s.RouterTemplateStorage.Get("my-router")
	c.Assert(err, check.IsNil)
	c.Assert(rtDB, check.DeepEquals, &rt)
}

func (s *RouterTemplateSuite) TestGetNotFound(c *check.C) {
	_, err := s.RouterTemplateStorage.Get("my-router")
	c.Assert(err, check.Equals, router.ErrRouterTemplateNotFound)
}

func (s *RouterTemplateSuite) TestList(c *check.C) {
	dbs, err := s.RouterTemplateStorage.List()
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.HasLen, 0)

	rt1 := router.RouterTemplate{
		Name: "my-router1",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	rt2 := router.RouterTemplate{
		Name: "my-router2",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err = s.RouterTemplateStorage.Save(rt1)
	c.Assert(err, check.IsNil)
	err = s.RouterTemplateStorage.Save(rt2)
	c.Assert(err, check.IsNil)
	dbs, err = s.RouterTemplateStorage.List()
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.DeepEquals, []router.RouterTemplate{rt1, rt2})
}

func (s *RouterTemplateSuite) TestR(c *check.C) {
	rt1 := router.RouterTemplate{
		Name: "my-router1",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	rt2 := router.RouterTemplate{
		Name: "my-router2",
		Type: "my-type",
		Config: map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": "e",
			},
		},
	}
	err := s.RouterTemplateStorage.Save(rt1)
	c.Assert(err, check.IsNil)
	err = s.RouterTemplateStorage.Save(rt2)
	c.Assert(err, check.IsNil)
	err = s.RouterTemplateStorage.Remove(rt1.Name)
	c.Assert(err, check.IsNil)
	dbs, err := s.RouterTemplateStorage.List()
	c.Assert(err, check.IsNil)
	c.Assert(dbs, check.DeepEquals, []router.RouterTemplate{rt2})
}

func (s *RouterTemplateSuite) TestRemoveNotFound(c *check.C) {
	err := s.RouterTemplateStorage.Remove("my-router")
	c.Assert(err, check.Equals, router.ErrRouterTemplateNotFound)
}
