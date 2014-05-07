// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"code.google.com/p/go.crypto/bcrypt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn   *db.Storage
	hashed string
	user   *auth.User
	team   *auth.Team
	token  *Token
}

var _ = gocheck.Suite(&S{})

var nativeScheme = NativeScheme{}

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("admin-team", "admin")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_native_test")
	s.conn, _ = db.Conn()
	s.user = &auth.User{Email: "timeredbull@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, gocheck.IsNil)
	s.hashed = s.user.Password
	s.token, _ = createToken(s.user, "123456")
	team := &auth.Team{Name: "cobrateam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	s.team = team
}

func (s *S) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = conn.Apps().Database.DropDatabase()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownTest(c *gocheck.C) {
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err := s.user.Update()
		c.Assert(err, gocheck.IsNil)
	}
	cost = 0
	tokenExpire = 0
}
