// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/app"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type appStorage interface {
	Create(*app.App) error
	Remove(*app.App) error
}

type AppQuotaSuite struct {
	SuiteHooks
	AppStorage      appStorage
	AppQuotaStorage appTypes.QuotaStorage
}

func (s *AppQuotaSuite) TestFindByAppName(c *check.C) {
	app := &app.App{Name: "myapp", Quota: appTypes.UnlimitedQuota}
	s.AppStorage.Create(app)
	quota, err := s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, -1)
}

func (s *AppQuotaSuite) TestFindByAppNameNotFound(c *check.C) {
	_, err := s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestIncInUse(c *check.C) {
	app := &app.App{Name: "myapp", Quota: appTypes.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.IncInUse("myapp", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 1)
	c.Assert(quota.Limit, check.Equals, 5)
	err = s.AppQuotaStorage.IncInUse("myapp", 2)
	c.Assert(err, check.IsNil)
	quota, err = s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 5)
}

func (s *AppQuotaSuite) TestIncInUseNotFound(c *check.C) {
	err := s.AppQuotaStorage.IncInUse("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestSetLimit(c *check.C) {
	app := &app.App{Name: "myapp", Quota: appTypes.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.SetLimit("myapp", 1)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, 1)
}

func (s *AppQuotaSuite) TestSetLimitNotFound(c *check.C) {
	err := s.AppQuotaStorage.SetLimit("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *AppQuotaSuite) TestSetInUse(c *check.C) {
	app := &app.App{Name: "myapp", Quota: appTypes.Quota{Limit: 5, InUse: 0}}
	s.AppStorage.Create(app)
	err := s.AppQuotaStorage.SetInUse("myapp", 3)
	c.Assert(err, check.IsNil)
	quota, err := s.AppQuotaStorage.FindByAppName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 5)
}

func (s *AppQuotaSuite) TestSetInUseNotFound(c *check.C) {
	err := s.AppQuotaStorage.SetInUse("myapp", 1)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}
