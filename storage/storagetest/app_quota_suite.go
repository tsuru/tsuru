// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/app"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

type appStorage interface {
	Create(*app.App) error
	Remove(*app.App) error
}

type AppQuotaSuite struct {
	SuiteHooks
	AppStorage      appStorage
	AppQuotaStorage quota.QuotaStorageWithSet
}

func (s *AppQuotaSuite) TestGet(c *check.C) {
	app := &app.App{Name: "myapp", Quota: quota.UnlimitedQuota}
	s.AppStorage.Create(app)
	quota, err := s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, -1)
}

func (s *AppQuotaSuite) TestGetNotFound(c *check.C) {
	_, err := s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestInc(c *check.C) {
	app := &app.App{Name: "myapp", Quota: quota.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.Inc("myapp", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 1)
	c.Assert(quota.Limit, check.Equals, 5)
	err = s.AppQuotaStorage.Inc("myapp", 2)
	c.Assert(err, check.IsNil)
	quota, err = s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 5)
}

func (s *AppQuotaSuite) TestIncNotFound(c *check.C) {
	err := s.AppQuotaStorage.Inc("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestSetLimit(c *check.C) {
	app := &app.App{Name: "myapp", Quota: quota.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.SetLimit("myapp", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, 1)
}

func (s *AppQuotaSuite) TestSetLimitNotFound(c *check.C) {
	err := s.AppQuotaStorage.SetLimit("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestSet(c *check.C) {
	app := &app.App{Name: "myapp", Quota: quota.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.Set("myapp", 3)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.Get("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 5)
}

func (s *AppQuotaSuite) TestSetNotFound(c *check.C) {
	err := s.AppQuotaStorage.Set("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}
