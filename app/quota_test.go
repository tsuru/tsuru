// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/quota"
	"gopkg.in/check.v1"
)

func (s *S) TestReserveUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnIncInUse: func(appName string, quantity int) error {
				c.Assert(appName, check.Equals, app.Name)
				c.Assert(quantity, check.Equals, 6)
				app.Quota.InUse += quantity
				return nil
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: 7, InUse: app.Quota.InUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 7, InUse: 6}
	err := qs.ReserveUnits(app.Name, 6)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByAppName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestReserveUnitsAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	err := qs.ReleaseUnits(app.Name, 6)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestReserveUnitsQuotaExceeded(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 6},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnIncInUse: func(appName string, quantity int) error {
				c.Assert(appName, check.Equals, "together")
				c.Assert(quantity, check.Equals, 2)
				return &quota.QuotaExceededError{Available: 1, Requested: 2}
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: 7, InUse: 6}, nil
			},
		},
	}
	err := qs.ReserveUnits(app.Name, 2)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestReserveUnitsUnlimitedQuota(c *check.C) {
	app := &App{Name: "together", Quota: quota.UnlimitedQuota, Router: "fake"}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnIncInUse: func(appName string, quantity int) error {
				c.Assert(appName, check.Equals, "together")
				c.Assert(quantity, check.Equals, 10)
				app.Quota.InUse += quantity
				return nil
			},

			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: -1, InUse: app.Quota.InUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: -1, InUse: 10}
	err := qs.ReserveUnits(app.Name, 10)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByAppName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestReleaseUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnIncInUse: func(appName string, quantity int) error {
				c.Assert(appName, check.Equals, "together")
				c.Assert(quantity, check.Equals, -6)
				app.Quota.InUse += quantity
				return nil
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: 7, InUse: app.Quota.InUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 7, InUse: 1}
	err := qs.ReleaseUnits(app.Name, 6)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByAppName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestReleaseUnreservedUnits(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnIncInUse: func(appName string, quantity int) error {
				c.Assert(appName, check.Equals, "together")
				c.Assert(quantity, check.Equals, -8)
				return nil
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: 7, InUse: 7}, nil
			},
		},
	}
	err := qs.ReleaseUnits(app.Name, 8)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, quota.ErrNoReservedUnits)
}

func (s *S) TestReleaseUnitsAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	err := qs.ReleaseUnits(app.Name, 6)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestChangeQuotaLimit(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 3, InUse: 3},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnSetLimit: func(appName string, limit int) error {
				c.Assert(appName, check.Equals, app.Name)
				c.Assert(limit, check.Equals, 30)
				app.Quota.Limit = limit
				return nil
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: app.Quota.Limit, InUse: 3}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 30, InUse: 3}
	err := qs.ChangeLimit(app.Name, 30)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByAppName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestChangeQuotaLimitToUnlimited(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 3, InUse: 2},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnSetLimit: func(appName string, limit int) error {
				c.Assert(appName, check.Equals, app.Name)
				c.Assert(limit, check.Equals, -1)
				app.Quota.Limit = limit
				return nil
			},
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				c.Assert(appName, check.Equals, app.Name)
				return &quota.Quota{Limit: app.Quota.Limit, InUse: 2}, nil
			},
		},
	}
	expected := quota.Quota{Limit: -1, InUse: 2}
	err := qs.ChangeLimit(app.Name, -5)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByAppName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestChangeQuotaLimitAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  quota.Quota{Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &quota.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*quota.Quota, error) {
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	err := qs.ChangeLimit(app.Name, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}
