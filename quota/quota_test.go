// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
}

var _ = check.Suite(&S{})

type namedItem string

func (n namedItem) GetName() string {
	return string(n)
}

func (s *S) TestInc(c *check.C) {
	inUse := 0
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSet: func(name string, quantity int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(quantity, check.Equals, 6)
				inUse = quantity
				return nil
			},
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: 7, InUse: inUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 7, InUse: 6}
	err := qs.Inc(context.TODO(), namedItem("myname"), 6)
	c.Assert(err, check.IsNil)
	quota, err := qs.Get(context.TODO(), namedItem("myname"))
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestIncAppNotFound(c *check.C) {
	myerr := errors.New("myerr")
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return nil, myerr
			},
		},
	}
	err := qs.Inc(context.TODO(), namedItem("myname"), -6)
	c.Assert(err, check.Equals, myerr)
}

func (s *S) TestIncQuotaExceeded(c *check.C) {
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSet: func(name string, quantity int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(quantity, check.Equals, 2)
				return &quota.QuotaExceededError{Available: 1, Requested: 2}
			},
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: 7, InUse: 6}, nil
			},
		},
	}
	err := qs.Inc(context.TODO(), namedItem("myname"), 2)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestIncUnlimitedQuota(c *check.C) {
	inUse := 0
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSet: func(name string, quantity int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(quantity, check.Equals, 10)
				inUse = quantity
				return nil
			},

			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: -1, InUse: inUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: -1, InUse: 10}
	err := qs.Inc(context.TODO(), namedItem("myname"), 10)
	c.Assert(err, check.IsNil)
	quota, err := qs.Get(context.TODO(), namedItem("myname"))
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestIncNegative(c *check.C) {
	inUse := 7
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSet: func(name string, quantity int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(quantity, check.Equals, 1)
				inUse = quantity
				return nil
			},
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: 7, InUse: inUse}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 7, InUse: 1}
	err := qs.Inc(context.TODO(), namedItem("myname"), -6)
	c.Assert(err, check.IsNil)
	quota, err := qs.Get(context.TODO(), namedItem("myname"))
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestIncNegativeTooLarge(c *check.C) {
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: 7, InUse: 7}, nil
			},
		},
	}
	err := qs.Inc(context.TODO(), namedItem("myname"), -8)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, quota.ErrNotEnoughReserved)
}

func (s *S) TestIncNegativeAppNotFound(c *check.C) {
	myerr := errors.New("myerr")
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnGet: func(name string) (*quota.Quota, error) {
				return nil, myerr
			},
		},
	}
	err := qs.Inc(context.TODO(), namedItem("myname"), -6)
	c.Assert(err, check.Equals, myerr)
}

func (s *S) TestSetLimit(c *check.C) {
	quotaLimit := 3
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSetLimit: func(name string, limit int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(limit, check.Equals, 30)
				quotaLimit = limit
				return nil
			},
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: quotaLimit, InUse: 3}, nil
			},
		},
	}
	expected := quota.Quota{Limit: 30, InUse: 3}
	err := qs.SetLimit(context.TODO(), namedItem("myname"), 30)
	c.Assert(err, check.IsNil)
	quota, err := qs.Get(context.TODO(), namedItem("myname"))
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestSetLimitToUnlimited(c *check.C) {
	quotaLimit := 3
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnSetLimit: func(name string, limit int) error {
				c.Assert(name, check.Equals, "myname")
				c.Assert(limit, check.Equals, -1)
				quotaLimit = limit
				return nil
			},
			OnGet: func(name string) (*quota.Quota, error) {
				c.Assert(name, check.Equals, "myname")
				return &quota.Quota{Limit: quotaLimit, InUse: 2}, nil
			},
		},
	}
	expected := quota.Quota{Limit: -1, InUse: 2}
	err := qs.SetLimit(context.TODO(), namedItem("myname"), -5)
	c.Assert(err, check.IsNil)
	quota, err := qs.Get(context.TODO(), namedItem("myname"))
	c.Assert(err, check.IsNil)
	c.Assert(*quota, check.DeepEquals, expected)
}

func (s *S) TestSetLimitAppNotFound(c *check.C) {
	myerr := errors.New("myerr")
	qs := &QuotaService[quota.QuotaItem]{
		Storage: &quota.MockQuotaStorage{
			OnGet: func(name string) (*quota.Quota, error) {
				return nil, myerr
			},
		},
	}
	err := qs.SetLimit(context.TODO(), namedItem("myname"), 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, myerr)
}
