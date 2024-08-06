// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn   *db.Storage
	hashed string
	user   *User
	team   *authTypes.Team
	server *authtest.SMTPServer
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_auth_test")
	s.conn, _ = db.Conn()
	config.Set("smtp:user", "root")
	var err error

	storagev2.Reset()

	servicemanager.TeamToken, err = TeamTokenService()
	c.Assert(err, check.IsNil)
	servicemanager.Team, err = TeamService()
	c.Assert(err, check.IsNil)
	servicemanager.AuthGroup, err = GroupService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	s.user = &User{Email: "timeredbull@globo.com", Password: "123456"}
	s.user.Create(context.TODO())
	s.hashed = s.user.Password
	s.team = &authTypes.Team{Name: "cobrateam"}
	u := authTypes.User(*s.user)
	svc, err := TeamService()
	c.Assert(err, check.IsNil)
	err = svc.Create(context.TODO(), s.team.Name, nil, &u)
	c.Assert(err, check.IsNil)
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err := s.user.Update(context.TODO())
		c.Assert(err, check.IsNil)
	}
}
