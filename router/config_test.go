// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

type ConfigSuite struct {
	getter ConfigGetter
}

func init() {
	check.Suite(&ConfigSuite{getter: &StaticConfigGetter{}})
	check.Suite(&ConfigSuite{getter: &templateConfigGetter{}})
}

func (s *ConfigSuite) SetUpTest(c *check.C) {
	Register("myrouter", func(name string, config ConfigGetter) (Router, error) {
		return nil, nil
	})

	routerConfig := map[string]interface{}{
		"str":       "str",
		"str-int":   "1",
		"str-float": "1.1",
		"int":       1,
		"float":     1.1,
		"bool":      true,
		"complex": map[string]interface{}{
			"a": "b",
			"c": map[string]interface{}{
				"d": 1,
			},
		},
	}
	config.Set("mine:type", "myrouter")
	config.Set("mine:str", "str")
	config.Set("mine:str-int", "1")
	config.Set("mine:str-float", "1.1")
	config.Set("mine:int", 1)
	config.Set("mine:float", 1.1)
	config.Set("mine:bool", true)
	config.Set("mine:complex:a", "b")
	config.Set("mine:complex:c:d", 1)

	switch g := s.getter.(type) {
	case *StaticConfigGetter:
		g.Prefix = "mine"
	case *templateConfigGetter:
		svc, err := TemplateService()
		c.Assert(err, check.IsNil)
		rt := router.RouterTemplate{
			Name:   "mine",
			Type:   "myrouter",
			Config: routerConfig,
		}
		err = svc.Create(rt)
		c.Assert(err, check.IsNil)
		dbRt, err := svc.Get(rt.Name)
		c.Assert(err, check.IsNil)
		g.template = *dbRt
	}
}

func (s *ConfigSuite) TearDownTest(c *check.C) {
	config.Unset("mine")
}

func (s *ConfigSuite) TestGetString(c *check.C) {
	v, err := s.getter.GetString("str")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, "str")
	v, err = s.getter.GetString("int")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, "1")
}

func (s *ConfigSuite) TestGetInt(c *check.C) {
	v, err := s.getter.GetInt("str-int")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, 1)
	v, err = s.getter.GetInt("int")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, 1)
}

func (s *ConfigSuite) TestGetFloat(c *check.C) {
	v, err := s.getter.GetFloat("str-float")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, 1.1)
	v, err = s.getter.GetFloat("float")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, 1.1)
	v, err = s.getter.GetFloat("int")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, 1.0)
}

func (s *ConfigSuite) TestGetBool(c *check.C) {
	v, err := s.getter.GetBool("bool")
	c.Assert(err, check.IsNil)
	c.Check(v, check.Equals, true)
}

func (s *ConfigSuite) TestGet(c *check.C) {
	v, err := s.getter.Get("complex")
	c.Assert(err, check.IsNil)
	c.Check(v, check.DeepEquals, map[string]interface{}{
		"a": "b",
		"c": map[string]interface{}{
			"d": 1,
		},
	})
}
