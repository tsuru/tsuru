// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"code.google.com/p/go.crypto/bcrypt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"io"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type hasKeyChecker struct{}

func (c *hasKeyChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasKey", Params: []string{"user", "key"}}
}

func (c *hasKeyChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should provide two parameters"
	}
	user, ok := params[0].(*User)
	if !ok {
		return false, "first parameter should be a user pointer"
	}
	content, ok := params[1].(string)
	if !ok {
		return false, "second parameter should be a string"
	}
	key := Key{Content: content}
	return user.HasKey(key), ""
}

var HasKey gocheck.Checker = &hasKeyChecker{}

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn    *db.Storage
	hashed  string
	user    *User
	team    *Team
	token   *Token
	gitRoot string
	gitHost string
	gitPort string
	gitProt string
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("auth:salt", "tsuru-salt")
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("admin-team", "admin")
	s.hashed = hashPassword("123")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_test")
	s.conn, _ = db.Conn()
	s.user = &User{Email: "timeredbull@globo.com", Password: "123456"}
	s.user.Create()
	s.token, _ = s.user.CreateToken("123456")
	team := &Team{Name: "cobrateam", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	s.team = team
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *gocheck.C) {
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err := s.user.Update()
		c.Assert(err, gocheck.IsNil)
	}
	config.Set("git:host", s.gitHost)
	config.Set("git:port", s.gitPort)
	config.Set("git:protocol", s.gitProt)
	salt = ""
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func (s *S) startGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}
