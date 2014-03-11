// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/quota"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"runtime"
	"sync"
)

func (s *S) TestReserveApp(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, gocheck.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 1)
}

func (s *S) TestReserveAppUserNotFound(c *gocheck.C) {
	user := User{Email: "hills@waaaat.com"}
	err := ReserveApp(&user)
	c.Assert(err, gocheck.Equals, ErrUserNotFound)
}

func (s *S) TestReserveAppAlwaysRefreshFromDatabase(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	user.InUse = 4
	err = ReserveApp(user)
	c.Assert(err, gocheck.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 1)
}

func (s *S) TestReserveAppQuotaExceeded(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 4},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(0))
	c.Assert(e.Requested, gocheck.Equals, uint(1))
}

func (s *S) TestReserveAppIsSafe(c *gocheck.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 10, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 10)
}

func (s *S) TestReleaseApp(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, gocheck.IsNil)
	err = ReleaseApp(user)
	c.Assert(err, gocheck.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 0)
}

func (s *S) TestReleaseAppUserNotFound(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := ReleaseApp(user)
	c.Assert(err, gocheck.Equals, ErrUserNotFound)
}

func (s *S) TestReleaseAppAlwaysRefreshFromDatabase(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReserveApp(user)
	c.Assert(err, gocheck.IsNil)
	user.InUse = 4
	err = ReleaseApp(user)
	c.Assert(err, gocheck.IsNil)
	user, err = GetUserByEmail(email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 0)
}

func (s *S) TestReleaseAppNonReserved(c *gocheck.C) {
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 4, InUse: 0},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": user.Email})
	err = ReleaseApp(user)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot release unreserved app")
}

func (s *S) TestReleaseAppIsSafe(c *gocheck.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))
	email := "seven@corp.globo.com"
	user := &User{
		Email: email, Password: "123456",
		Quota: quota.Quota{Limit: 10, InUse: 10},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.InUse, gocheck.Equals, 0)
}
