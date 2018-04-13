// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"runtime"
	"sync"

	"github.com/globalsign/mgo/bson"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestReserveUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7},
		Router: "fake",
	}
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	err := qs.ReserveUnits(app.Quota, 6)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 6)
}

// func (s *S) TestReserveUnitsAppNotFound(c *check.C) {
// 	app := App{
// 		Name:   "together",
// 		Quota:  appTypes.Quota{Limit: 7},
// 		Router: "fake",
// 	}
// 	err := reserveUnits(&app, 6)
// 	c.Assert(err, check.Equals, ErrAppNotFound)
// }

func (s *S) TestReserveUnitsQuotaExceeded(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 7},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	err := qs.ReserveUnits(app.Quota, 6)
	c.Assert(err, check.IsNil)
	err = qs.ReserveUnits(app.Quota, 2)
	c.Assert(err, check.NotNil)
	e, ok := err.(*appTypes.AppQuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestReserveUnitsUnlimitedQuota(c *check.C) {
	app := &App{Name: "together", Quota: &appTypes.AppQuota{AppName: "together", Limit: -1}, Router: "fake"}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	err := qs.ReserveUnits(app.Quota, 10)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 10)
}

func (s *S) TestReserveUnitsIsAtomic(c *check.C) {
	ncpu := runtime.NumCPU()
	originalMaxProcs := runtime.GOMAXPROCS(ncpu)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	app := &App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 60},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qs.ReserveUnits(app.Quota, 7)
		}()
	}
	wg.Wait()
	c.Assert(app.Quota.InUse, check.Equals, 56)
}

func (s *S) TestReleaseUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	err := qs.ReleaseUnits(app.Quota, 6)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReleaseUnreservedUnits(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	err := qs.ReleaseUnits(app.Quota, 8)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrNoReservedUnits)
}

func (s *S) TestReleaseUnitsIsAtomic(c *check.C) {
	ncpu := runtime.NumCPU()
	originalMaxProcs := runtime.GOMAXPROCS(ncpu)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	app := &App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 60, InUse: 60},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qs.ReleaseUnits(app.Quota, 7)
		}()
	}
	wg.Wait()
	c.Assert(app.Quota.InUse, check.Equals, 4)
}

func (s *S) TestChangeQuotaIsAtomic(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  &appTypes.AppQuota{AppName: "together", Limit: 3, InUse: 3},
		Router: "fake",
	}
	qs := &quotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(service appTypes.AppQuotaService, quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
		},
		mutex: &sync.Mutex{},
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qs.ReleaseUnits(app.Quota, 7)
		}()
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := ChangeQuota(app, 2)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "new limit is lesser than the current allocated value")
}

// func (s *S) TestReleaseUnitsAppNotFound(c *check.C) {
// 	app := App{
// 		Name:   "together",
// 		Quota:  quota.Quota{Limit: 7, InUse: 7},
// 		Router: "fake",
// 	}
// 	err := releaseUnits(&app, 6)
// 	c.Assert(err, check.Equals, ErrAppNotFound)
// }

// func (s *S) TestChangeQuota(c *check.C) {
// 	app := &App{
// 		Name:   "together",
// 		Quota:  quota.Quota{Limit: 3, InUse: 3},
// 		Router: "fake",
// 	}
// 	s.conn.Apps().Insert(app)
// 	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
// 	err := ChangeQuota(app, 30)
// 	c.Assert(err, check.IsNil)
// 	app, err = GetByName(app.Name)
// 	c.Assert(err, check.IsNil)
// 	c.Assert(app.Quota.InUse, check.Equals, 3)
// 	c.Assert(app.Quota.Limit, check.Equals, 30)
// }

// func (s *S) TestChangeQuotaUnlimited(c *check.C) {
// 	app := &App{
// 		Name:   "together",
// 		Quota:  quota.Quota{Limit: 3, InUse: 2},
// 		Router: "fake",
// 	}
// 	s.conn.Apps().Insert(app)
// 	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
// 	err := ChangeQuota(app, -5)
// 	c.Assert(err, check.IsNil)
// 	app, err = GetByName(app.Name)
// 	c.Assert(err, check.IsNil)
// 	c.Assert(app.Quota.InUse, check.Equals, 2)
// 	c.Assert(app.Quota.Limit, check.Equals, -1)
// }

// func (s *S) TestChangeQuotaAppNotFound(c *check.C) {
// 	app := &App{
// 		Name:   "together",
// 		Quota:  quota.Quota{Limit: 3, InUse: 3},
// 		Router: "fake",
// 	}
// 	err := ChangeQuota(app, 20)
// 	c.Assert(err, check.NotNil)
// 	c.Assert(err, check.Equals, mgo.ErrNotFound)
// }
