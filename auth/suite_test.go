// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn    *db.Storage
	hashed  string
	user    *User
	team    *Team
	server  *authtest.SMTPServer
	gitRoot string
	gitHost string
	gitPort string
	gitProt string
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("admin-team", "admin")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_test")
	s.conn, _ = db.Conn()
	s.user = &User{Email: "timeredbull@globo.com", Password: "123456"}
	s.user.Create()
	s.hashed = s.user.Password
	team := &Team{Name: "cobrateam", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	s.team = team
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	config.Set("smtp:password", "123456")
	config.Set("repo-manager", "fake")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
	s.server.Stop()
}

func (s *S) SetUpTest(c *check.C) {
	repositorytest.Reset()
}

func (s *S) TearDownTest(c *check.C) {
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err := s.user.Update()
		c.Assert(err, check.IsNil)
	}
	config.Set("git:host", s.gitHost)
	config.Set("git:port", s.gitPort)
	config.Set("git:protocol", s.gitProt)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}
