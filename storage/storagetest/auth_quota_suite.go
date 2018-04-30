// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/auth"
	authTypes "github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type AuthQuotaSuite struct {
	SuiteHooks
	UserStorage      authTypes.UserStorage
	AuthQuotaStorage authTypes.QuotaStorage
	AuthQuotaService authTypes.QuotaService
}

func (s *AuthQuotaSuite) TestFindByUserEmail(c *check.C) {
	user := &auth.User{Email: "example@example.com", Quota: authTypes.Quota{Limit: -1, InUse: 0}}
	s.UserStorage.Create(user)
	defer s.UserStorage.Remove(user)
	quota, err := s.AuthQuotaStorage.FindByUserEmail("example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, -1)
}

func (s *AuthQuotaSuite) TestFindByAppNameNotFound(c *check.C) {
	_, err := s.AuthQuotaStorage.FindByUserEmail("myapp")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *AuthQuotaSuite) TestIncInUse(c *check.C) {
	user := &auth.User{Email: "example@example.com", Quota: authTypes.Quota{Limit: 5, InUse: 0}}
	s.UserStorage.Create(user)
	defer s.UserStorage.Remove(user)
	err := s.AuthQuotaStorage.IncInUse("example@example.com", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AuthQuotaStorage.FindByUserEmail("example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 1)
	c.Assert(quota.Limit, check.Equals, 5)
	err = s.AuthQuotaStorage.IncInUse("example@example.com", 2)
	c.Assert(err, check.IsNil)
	quota, err = s.AuthQuotaStorage.FindByUserEmail("example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 5)
}

func (s *AuthQuotaSuite) TestIncInUseNotFound(c *check.C) {
	err := s.AuthQuotaStorage.IncInUse("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *AuthQuotaSuite) TestSetLimit(c *check.C) {
	user := &auth.User{Email: "example@example.com", Quota: authTypes.Quota{Limit: 5, InUse: 0}}
	s.UserStorage.Create(user)
	defer s.UserStorage.Remove(user)
	err := s.AuthQuotaStorage.SetLimit("example@example.com", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AuthQuotaStorage.FindByUserEmail("example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, 1)
}

func (s *AuthQuotaSuite) TestSetLimitNotFound(c *check.C) {
	err := s.AuthQuotaStorage.SetLimit("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}
