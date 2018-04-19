// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"runtime"
	"sync"

	"github.com/globalsign/mgo"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func defaultOnFindByUserEmail(email string) (*authTypes.AuthQuota, error) {
	return nil, nil
}

func (s *S) TestReserveApp(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, *quota)
				c.Assert(quantity, check.Equals, 1)
				return nil
			},
			OnFindByUserEmail: defaultOnFindByUserEmail,
		},
	}
	expected := authTypes.AuthQuota{Limit: 4, InUse: 1}
	err := qs.ReserveApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota, check.Equals, expected)
}

func (s *S) TestReserveAppUserNotFound(c *check.C) {
	user := User{Email: "hills@waaaat.com"}
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.AuthQuota, error) {
				c.Assert(email, check.Equals, user.Email)
				return nil, authTypes.ErrUserNotFound
			},
		},
	}
	err := qs.ReserveApp(user.Email, &user.Quota)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestReserveAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, quota)
				c.Assert(quantity, check.Equals, 1)
				return nil
			},
		},
	}
	user.Quota.InUse = 4
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	err = qs.ReserveApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReserveAppQuotaExceeded(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 4},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{},
	}
	err = qs.ReserveApp(user.Email, &user.Quota)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
}

func (s *S) TestReserveAppIsSafe(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(runtime.NumCPU())
	defer runtime.GOMAXPROCS(originalMaxProcs)
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 10, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, quota)
				c.Assert(quantity, check.Equals, 1)
				return nil
			},
		},
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qs.ReserveApp(user.Email, &user.Quota)
		}()
	}
	wg.Wait()
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 10)
}

func (s *S) TestReleaseApp(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, quota)
				return nil
			},
		},
	}
	err = qs.ReserveApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	err = qs.ReleaseApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *S) TestReleaseAppUserNotFound(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{},
	}
	err := qs.ReleaseApp(user.Email, &user.Quota)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestReleaseAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, quota)
				return nil
			},
		},
	}
	err = qs.ReserveApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	user.Quota.InUse = 4
	err = qs.ReleaseApp(user.Email, &user.Quota)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *S) TestReleaseAppNonReserved(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()

	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{},
	}
	err = qs.ReleaseApp(user.Email, &user.Quota)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot release unreserved app")
}

func (s *S) TestReleaseAppIsSafe(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(runtime.NumCPU())
	defer runtime.GOMAXPROCS(originalMaxProcs)
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 10, InUse: 10},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	var wg sync.WaitGroup
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnIncInUse: func(email string, quota *authTypes.AuthQuota, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(user.Quota, check.DeepEquals, quota)
				c.Assert(quantity, check.Equals, -1)
				return nil
			},
		},
	}
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qs.ReleaseApp(user.Email, &user.Quota)
		}()
	}
	wg.Wait()
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *S) TestChangeQuota(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 3},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnSetLimit: func(email string, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(quantity, check.Equals, 40)
				return nil
			},
		},
	}
	err = qs.ChangeQuota(user.Email, &user.Quota, 40)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 3)
	c.Assert(user.Quota.Limit, check.Equals, 40)
}

func (s *S) TestChangeQuotaUnlimited(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 3},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{
			OnSetLimit: func(email string, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(quantity, check.Equals, -40)
				return nil
			},
		},
	}
	err = qs.ChangeQuota(user.Email, &user.Quota, -40)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 3)
	c.Assert(user.Quota.Limit, check.Equals, -1)
}

func (s *S) TestChangeQuotaLessThanInUse(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 4},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{},
	}
	err = qs.ChangeQuota(user.Email, &user.Quota, 3)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "new limit is lesser than the current allocated value")
}

func (s *S) TestChangeQuotaUserNotFound(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.AuthQuota{Limit: 4, InUse: 4},
	}
	qs := &authQuotaService{
		storage: &authTypes.MockAuthQuotaStorage{},
	}
	err := qs.ChangeQuota(user.Email, &user.Quota, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}
