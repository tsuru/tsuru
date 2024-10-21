// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

type userStorage interface {
	Create(*auth.User) error
	Remove(*auth.User) error
}

type UserQuotaSuite struct {
	SuiteHooks
	UserStorage      userStorage
	UserQuotaStorage quota.QuotaStorage
	UserQuotaService quota.LegacyQuotaService
}

func (s *UserQuotaSuite) TestGet(c *check.C) {
	user := &auth.User{Email: "example@example.com", Quota: quota.UnlimitedQuota}
	s.UserStorage.Create(user)
	quota, err := s.UserQuotaStorage.Get(context.TODO(), "example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, -1)
}

func (s *UserQuotaSuite) TestGetNotFound(c *check.C) {
	_, err := s.UserQuotaStorage.Get(context.TODO(), "myapp")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, quota.ErrQuotaNotFound)
}

func (s *UserQuotaSuite) TestSetLimit(c *check.C) {
	user := &auth.User{Email: "example@example.com", Quota: quota.Quota{Limit: 5}}
	err := s.UserStorage.Create(user)
	c.Assert(err, check.IsNil)
	err = s.UserQuotaStorage.SetLimit(context.TODO(), "example@example.com", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.UserQuotaStorage.Get(context.TODO(), "example@example.com")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, 1)
}

func (s *UserQuotaSuite) TestSetLimitNotFound(c *check.C) {
	err := s.UserQuotaStorage.SetLimit(context.TODO(), "myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, quota.ErrQuotaNotFound)
}
