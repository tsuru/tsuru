// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn    *db.Storage
	hashed  string
	user    *User
	team    *authTypes.Team
	server  *authtest.SMTPServer
	gitHost string
	gitPort string
	gitProt string
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_auth_test")
	s.conn, _ = db.Conn()
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
	config.Set("smtp:user", "root")
	config.Set("repo-manager", "fake")
	var err error
	servicemanager.TeamToken, err = TeamTokenService()
	c.Assert(err, check.IsNil)
	servicemanager.Team, err = TeamService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Users().Database.DropDatabase()
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.user = &User{Email: "timeredbull@globo.com", Password: "123456"}
	s.user.Create()
	s.hashed = s.user.Password
	s.team = &authTypes.Team{Name: "cobrateam"}
	u := authTypes.User(*s.user)
	svc, err := TeamService()
	c.Assert(err, check.IsNil)
	err = svc.Create(s.team.Name, nil, &u)
	c.Assert(err, check.IsNil)
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	repositorytest.Reset()
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Stop()
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err := s.user.Update()
		c.Assert(err, check.IsNil)
	}
	config.Set("git:host", s.gitHost)
	config.Set("git:port", s.gitPort)
	config.Set("git:protocol", s.gitProt)
}
