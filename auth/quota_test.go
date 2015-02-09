// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"runtime"
	"sync"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestReserveApp(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReserveAppUserNotFound(c *check.C) {
	user := User{Email: "hills@waaaat.com"}
	err := ReserveApp(&user)
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestReserveAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	user.InUse = 4
	err = ReserveApp(user)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 1)
}

func (s *S) TestReserveAppQuotaExceeded(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 4},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
}

func (s *S) TestReserveAppIsSafe(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 10, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ReserveApp(user)
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
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, check.IsNil)
	err = ReleaseApp(user)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *S) TestReleaseAppUserNotFound(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := ReleaseApp(user)
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestReleaseAppAlwaysRefreshFromDatabase(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, check.IsNil)
	user.InUse = 4
	err = ReleaseApp(user)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *S) TestReleaseAppNonReserved(c *check.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReleaseApp(user)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot release unreserved app")
}

func (s *S) TestReleaseAppIsSafe(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 10, InUse: 10},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ReleaseApp(user)
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
		Quota: quota.Quota{Limit: 4, InUse: 3},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ChangeQuota(user, 40)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 3)
	c.Assert(user.Quota.Limit, check.Equals, 40)
}

func (s *S) TestChangeQuotaUnlimited(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 3},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ChangeQuota(user, -40)
	c.Assert(err, check.IsNil)
	user, err = GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.InUse, check.Equals, 3)
	c.Assert(user.Quota.Limit, check.Equals, -1)
}

func (s *S) TestChangeQuotaLessThanInUse(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 4},
	}
	err := user.Create()
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ChangeQuota(user, 3)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "new limit is lesser than the current allocated value")
}

func (s *S) TestChangeQuotaUserNotFound(c *check.C) {
	user := &User{
		Email: "seven@corp.globo.com", Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 4},
	}
	err := ChangeQuota(user, 20)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}
