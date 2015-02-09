// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"runtime"
	"sync"

	"github.com/tsuru/tsuru/quota"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestReserveUnits(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := reserveUnits(app, 6)
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 6)
}

func (s *S) TestReserveUnitsAppNotFound(c *check.C) {
	app := App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7},
	}
	err := reserveUnits(&app, 6)
	c.Assert(err, check.Equals, ErrAppNotFound)
}

func (s *S) TestReserveUnitsQuotaExceeded(c *check.C) {
	app := App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := reserveUnits(&app, 6)
	c.Assert(err, check.IsNil)
	err = reserveUnits(&app, 2)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Requested, check.Equals, uint(2))
	c.Assert(e.Available, check.Equals, uint(1))
}

func (s *S) TestReserveUnitsUnlimitedQuota(c *check.C) {
	app := &App{Name: "together", Quota: quota.Unlimited}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := reserveUnits(app, 6)
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 6)
}

func (s *S) TestReserveUnitsIsAtomic(c *check.C) {
	ncpu := runtime.NumCPU()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(ncpu))
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 40},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reserveUnits(app, 3)
		}()
	}
	wg.Wait()
	app, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 39)
}

func (s *S) TestReleaseUnits(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7, InUse: 7},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := releaseUnits(app, 6)
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReleaseUnreservedUnits(c *check.C) {
	app := App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7, InUse: 7},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := releaseUnits(&app, 8)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Not enough reserved units")
}

func (s *S) TestReleaseUnitsIsAtomic(c *check.C) {
	ncpu := runtime.NumCPU()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(ncpu))
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 40, InUse: 40},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			releaseUnits(app, 3)
		}()
	}
	wg.Wait()
	app, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReleaseUnitsAppNotFound(c *check.C) {
	app := App{
		Name:  "together",
		Quota: quota.Quota{Limit: 7, InUse: 7},
	}
	err := releaseUnits(&app, 6)
	c.Assert(err, check.Equals, ErrAppNotFound)
}

func (s *S) TestChangeQuota(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 3, InUse: 3},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := ChangeQuota(app, 30)
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 3)
	c.Assert(app.Quota.Limit, check.Equals, 30)
}

func (s *S) TestChangeQuotaUnlimited(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 3, InUse: 2},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := ChangeQuota(app, -5)
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 2)
	c.Assert(app.Quota.Limit, check.Equals, -1)
}

func (s *S) TestChangeQuotaLessThanInUse(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 3, InUse: 3},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := ChangeQuota(app, 2)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "new limit is lesser than the current allocated value")
}

func (s *S) TestChangeQuotaAppNotFound(c *check.C) {
	app := &App{
		Name:  "together",
		Quota: quota.Quota{Limit: 3, InUse: 3},
	}
	err := ChangeQuota(app, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}
