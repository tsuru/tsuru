// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn   *db.Storage
	user   *auth.User
	team   *types.Team
	server *authtest.SMTPServer
	token  auth.Token
}

var _ = check.Suite(&S{})

var nativeScheme = NativeScheme{}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_native_test")
	var err error
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	config.Set("smtp:password", "123456")
}

func (s *S) SetUpTest(c *check.C) {
	s.conn, _ = db.Conn()
	s.user = &auth.User{Email: "timeredbull@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	s.team = &types.Team{Name: "cobrateam"}
	err = auth.TeamService().Insert(*s.team)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Users().Database)
	c.Assert(err, check.IsNil)
	s.conn.Close()
	cost = 0
	tokenExpire = 0
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}
