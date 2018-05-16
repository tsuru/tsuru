// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestReserveApp(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnIncInUse: func(email string, quantity int) error {
				c.Assert(email, check.Equals, user.Email)
				c.Assert(quantity, check.Equals, 1)
				user.Quota.InUse++
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.InUse, InUse: user.Quota.Limit}, nil
			},
		},
	}
	expected := authTypes.Quota{Limit: 4, InUse: 1}
	err := qs.ReserveApp(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota, check.Equals, expected)
}

func (s *S) TestReserveAppUserNotFound(c *check.C) {
	user := User{Email: "hills@waaaat.com"}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return nil, authTypes.ErrUserNotFound
			},
		},
	}
	err := qs.ReserveApp(user.Email)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestReserveAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 1},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnIncInUse: func(email string, quantity int) error {
				c.Assert(email, check.Equals, user.Email)
				c.Assert(quantity, check.Equals, 1)
				user, err = GetUserByEmail(email)
				c.Assert(err, check.IsNil)
				user.Quota.InUse += quantity
				user.Update()
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				user, err = GetUserByEmail(email)
				c.Assert(err, check.IsNil)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	user.Quota.InUse = 4
	err = qs.ReserveApp(user.Email)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByUserEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 2)
}

func (s *S) TestReserveAppQuotaExceeded(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err := qs.ReserveApp(user.Email)
	e, ok := err.(*authTypes.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
}

func (s *S) TestReleaseApp(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnIncInUse: func(email string, quantity int) error {
				c.Assert(email, check.Equals, user.Email)
				user.Quota.InUse += quantity
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err := qs.ReserveApp(user.Email)
	c.Assert(err, check.IsNil)
	err = qs.ReleaseApp(user.Email)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByUserEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
	c.Assert(quota.Limit, check.Equals, 4)
}

func (s *S) TestReleaseAppUserNotFound(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return nil, authTypes.ErrUserNotFound
			},
		},
	}
	err := qs.ReleaseApp(user.Email)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestReleaseAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 1},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnIncInUse: func(email string, quantity int) error {
				c.Assert(email, check.Equals, user.Email)
				user, err = GetUserByEmail(user.Email)
				c.Assert(quantity, check.Equals, -1)
				c.Assert(err, check.IsNil)
				user.Quota.InUse += quantity
				user.Update()
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				user, err = GetUserByEmail(user.Email)
				c.Assert(err, check.IsNil)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	user.Quota.InUse = 4
	err = qs.ReleaseApp(user.Email)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByUserEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 0)
}

func (s *S) TestReleaseAppNonReserved(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: authTypes.Quota{Limit: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err := qs.ReleaseApp(user.Email)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot release unreserved app")
}

func (s *S) TestChangeQuotaLimit(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 3},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnSetLimit: func(email string, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(quantity, check.Equals, 40)
				user.Quota.Limit = 40
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err = qs.ChangeLimit(user.Email, 40)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByUserEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, 40)
}

func (s *S) TestChangeQuotaLimitUnlimited(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 3},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnSetLimit: func(email string, quantity int) error {
				c.Assert(email, check.Equals, "seven@corp.globo.com")
				c.Assert(quantity, check.Equals, -1)
				user.Quota.Limit = -1
				return nil
			},
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err := qs.ChangeLimit(user.Email, -40)
	c.Assert(err, check.IsNil)
	quota, err := qs.FindByUserEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(quota.InUse, check.Equals, 3)
	c.Assert(quota.Limit, check.Equals, -1)
}

func (s *S) TestChangeQuotaLimitLessThanInUse(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return &authTypes.Quota{Limit: user.Quota.Limit, InUse: user.Quota.InUse}, nil
			},
		},
	}
	err := qs.ChangeLimit(user.Email, 3)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "New limit is lesser than the current allocated value")
}

func (s *S) TestChangeQuotaLimitUserNotFound(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: authTypes.Quota{Limit: 4, InUse: 4},
	}
	qs := &userQuotaService{
		storage: &authTypes.MockQuotaStorage{
			OnFindByUserEmail: func(email string) (*authTypes.Quota, error) {
				c.Assert(email, check.Equals, user.Email)
				return nil, authTypes.ErrUserNotFound
			},
		},
	}
	err := qs.ChangeLimit(user.Email, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}
