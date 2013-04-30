// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

var _ = gocheck.Suite(Suite{})

type Suite struct{}

func (Suite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_quota_tests")
}

func (Suite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (Suite) TestCreate(c *gocheck.C) {
	err := Create("user@tsuru.io", 10)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var u usage
	err = conn.Quota().Find(bson.M{"user": "user@tsuru.io"}).One(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Quota().Remove(bson.M{"user": "user@tsuru.io"})
	c.Assert(u.User, gocheck.Equals, "user@tsuru.io")
	c.Assert(u.Limit, gocheck.Equals, uint(10))
	c.Assert(u.Items, gocheck.HasLen, 0)
}

func (Suite) TestDuplicateQuota(c *gocheck.C) {
	err := Create("user@tsuru.io", 10)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer conn.Quota().Remove(bson.M{"user": "user@tsuru.io"})
	err = Create("user@tsuru.io", 50)
	c.Assert(err, gocheck.Equals, ErrQuotaAlreadyExists)
}

func (Suite) TestDelete(c *gocheck.C) {
	err := Create("home@dreamtheater.com", 3)
	c.Assert(err, gocheck.IsNil)
	err = Delete("home@dreamtheater.com")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	count, err := conn.Quota().Find(bson.M{"user": "home@dreamtheater.com"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (Suite) TestDeleteQuotaNotFound(c *gocheck.C) {
	err := Delete("home@dreamtheater.com")
	c.Assert(err, gocheck.Equals, ErrQuotaNotFound)
}
