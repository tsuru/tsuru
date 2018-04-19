// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func defaultOnFindByAppName(appName string) (*appTypes.AppQuota, error) {
	return nil, nil
}

func (s *S) TestReserveUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 6}
	err := qs.ReserveUnits(&app.Quota, 6)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

func (s *S) TestReserveUnitsAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*appTypes.AppQuota, error) {
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	err := qs.ReleaseUnits(&app.Quota, 6)
	c.Assert(err, check.Equals, ErrAppNotFound)
}

func (s *S) TestReserveUnitsQuotaExceeded(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7},
		Router: "fake",
	}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	err := qs.ReserveUnits(&app.Quota, 6)
	c.Assert(err, check.IsNil)
	err = qs.ReserveUnits(&app.Quota, 2)
	c.Assert(err, check.NotNil)
	e, ok := err.(*appTypes.AppQuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(1))
	c.Assert(e.Requested, check.Equals, uint(2))
}

func (s *S) TestReserveUnitsUnlimitedQuota(c *check.C) {
	app := &App{Name: "together", Quota: appTypes.AppQuota{AppName: "together", Limit: -1}, Router: "fake"}
	// s.conn.Apps().Insert(app)
	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: -1, InUse: 10}
	err := qs.ReserveUnits(&app.Quota, 10)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

// func (s *S) TestReserveUnitsIsAtomic(c *check.C) {
// 	ncpu := runtime.NumCPU()
// 	originalMaxProcs := runtime.GOMAXPROCS(ncpu)
// 	defer runtime.GOMAXPROCS(originalMaxProcs)
// 	app := &App{
// 		Name:   "together",
// 		Quota:  appTypes.AppQuota{AppName: "together", Limit: 60},
// 		Router: "fake",
// 	}
// 	// s.conn.Apps().Insert(app)
// 	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
// 	qs := &appQuotaService{
// 		storage: &appTypes.MockAppQuotaStorage{
// 			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
// 				c.Assert(quota.AppName, check.Equals, "together")
// 				return nil
// 			},
// 			OnFindByAppName: defaultOnFindByAppName,
// 		},
// 	}
// 	var wg sync.WaitGroup
// 	for i := 0; i < 30; i++ {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			qs.ReserveUnits(&app.Quota, 7)
// 		}()
// 	}
// 	wg.Wait()
// 	c.Assert(app.Quota.InUse, check.Equals, 56)
// }

func (s *S) TestReleaseUnits(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 1}
	err := qs.ReleaseUnits(&app.Quota, 6)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

func (s *S) TestReleaseUnreservedUnits(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
				c.Assert(quota.AppName, check.Equals, "together")
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	err := qs.ReleaseUnits(&app.Quota, 8)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, appTypes.ErrNoReservedUnits)
}

// func (s *S) TestReleaseUnitsIsAtomic(c *check.C) {
// 	ncpu := runtime.NumCPU()
// 	originalMaxProcs := runtime.GOMAXPROCS(ncpu)
// 	defer runtime.GOMAXPROCS(originalMaxProcs)
// 	app := &App{
// 		Name:   "together",
// 		Quota:  appTypes.AppQuota{AppName: "together", Limit: 60, InUse: 60},
// 		Router: "fake",
// 	}
// 	// s.conn.Apps().Insert(app)
// 	// defer s.conn.Apps().Remove(bson.M{"name": app.Name})
// 	qs := &appQuotaService{
// 		storage: &appTypes.MockAppQuotaStorage{
// 			OnIncInUse: func(quota *appTypes.AppQuota, quantity int) error {
// 				c.Assert(quota.AppName, check.Equals, "together")
// 				return nil
// 			},
// 			OnFindByAppName: defaultOnFindByAppName,
// 		},
// 	}
// 	var wg sync.WaitGroup
// 	for i := 0; i < 20; i++ {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			qs.ReleaseUnits(&app.Quota, 7)
// 		}()
// 	}
// 	wg.Wait()
// 	c.Assert(app.Quota.InUse, check.Equals, 4)
// }

func (s *S) TestReleaseUnitsAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*appTypes.AppQuota, error) {
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7}
	err := qs.ReleaseUnits(&app.Quota, 6)
	c.Assert(err, check.Equals, ErrAppNotFound)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

func (s *S) TestChangeQuotaLimit(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 3, InUse: 3},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnSetLimit: func(appName string, limit int) error {
				c.Assert(appName, check.Equals, app.Name)
				c.Assert(limit, check.Equals, 3)
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: 30, InUse: 3}
	err := qs.ChangeLimit(&app.Quota, 30)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

func (s *S) TestChangeQuotaLimitToUnlimited(c *check.C) {
	app := &App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 3, InUse: 2},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnSetLimit: func(appName string, limit int) error {
				c.Assert(appName, check.Equals, app.Name)
				c.Assert(limit, check.Equals, 3)
				return nil
			},
			OnFindByAppName: defaultOnFindByAppName,
		},
	}
	expected := appTypes.AppQuota{AppName: "together", Limit: -1, InUse: 2}
	err := qs.ChangeLimit(&app.Quota, -5)
	c.Assert(err, check.IsNil)
	c.Assert(app.Quota, check.DeepEquals, expected)
}

func (s *S) TestChangeQuotaLimitAppNotFound(c *check.C) {
	app := App{
		Name:   "together",
		Quota:  appTypes.AppQuota{AppName: "together", Limit: 7, InUse: 7},
		Router: "fake",
	}
	qs := &appQuotaService{
		storage: &appTypes.MockAppQuotaStorage{
			OnFindByAppName: func(appName string) (*appTypes.AppQuota, error) {
				return nil, appTypes.ErrAppNotFound
			},
		},
	}
	err := qs.ChangeLimit(&app.Quota, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrAppNotFound)
}
