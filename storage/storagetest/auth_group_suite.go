// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type AuthGroupSuite struct {
	SuiteHooks
	AuthGroupStorage auth.GroupStorage
}

func (s *AuthGroupSuite) TestAddRole(c *check.C) {
	err := s.AuthGroupStorage.AddRole("g1", "r1", "v1")
	c.Assert(err, check.IsNil)
	err = s.AuthGroupStorage.AddRole("g1", "r1", "v1")
	c.Assert(err, check.IsNil)
	groups, err := s.AuthGroupStorage.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []auth.Group{
		{
			Name: "g1",
			Roles: []auth.RoleInstance{
				{
					Name:         "r1",
					ContextValue: "v1",
				},
			},
		},
	})

	err = s.AuthGroupStorage.AddRole("g1", "r2", "v1")
	c.Assert(err, check.IsNil)
	groups, err = s.AuthGroupStorage.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []auth.Group{
		{
			Name: "g1",
			Roles: []auth.RoleInstance{
				{
					Name:         "r1",
					ContextValue: "v1",
				},
				{
					Name:         "r2",
					ContextValue: "v1",
				},
			},
		},
	})
}

func (s *AuthGroupSuite) TestRemoveRole(c *check.C) {
	err := s.AuthGroupStorage.AddRole("g1", "r1", "v1")
	c.Assert(err, check.IsNil)
	err = s.AuthGroupStorage.RemoveRole("g1", "r1", "v1")
	c.Assert(err, check.IsNil)
	groups, err := s.AuthGroupStorage.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.DeepEquals, []auth.Group{
		{
			Name:  "g1",
			Roles: []auth.RoleInstance{},
		},
	})
}

func (s *AuthGroupSuite) TestList(c *check.C) {
	groups, err := s.AuthGroupStorage.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 0)
	err = s.AuthGroupStorage.AddRole("g1", "r1", "v1")
	c.Assert(err, check.IsNil)
	err = s.AuthGroupStorage.AddRole("g2", "r1", "v1")
	c.Assert(err, check.IsNil)
	groups, err = s.AuthGroupStorage.List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 2)
	groups, err = s.AuthGroupStorage.List(context.TODO(), []string{})
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 0)
	groups, err = s.AuthGroupStorage.List(context.TODO(), []string{"g1", "g2", "gn"})
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 2)
	groups, err = s.AuthGroupStorage.List(context.TODO(), []string{"g1"})
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 1)
	c.Assert(groups[0].Name, check.Equals, "g1")
	groups, err = s.AuthGroupStorage.List(context.TODO(), []string{"g2"})
	c.Assert(err, check.IsNil)
	c.Assert(groups, check.HasLen, 1)
	c.Assert(groups[0].Name, check.Equals, "g2")
}
