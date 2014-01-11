// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"code.google.com/p/go.crypto/bcrypt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	ttesting "github.com/globocom/tsuru/testing"
	"io"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"os"
	"path/filepath"
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
	conn    *db.TsrStorage
	hashed  string
	user    *User
	team    *Team
	token   *Token
	server  *ttesting.SMTPServer
	gitRoot string
	gitHost string
	gitPort string
	gitProt string
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("admin-team", "admin")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_auth_test")
	s.conn, _ = db.NewStorage()
	s.user = &User{Email: "timeredbull@globo.com", Password: "123456"}
	s.user.Create()
	s.hashed = s.user.Password
	s.token, _ = s.user.CreateToken("123456")
	team := &Team{Name: "cobrateam", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	s.team = team
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
	s.server, err = ttesting.NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	config.Set("smtp:password", "123456")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	conn, err := db.NewStorage()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	err = conn.Apps().Database.DropDatabase()
	c.Assert(err, gocheck.IsNil)
	s.server.Stop()
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
	cost = 0
	tokenExpire = 0
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}

type testHandler struct {
	body    [][]byte
	method  []string
	url     []string
	content string
	header  []http.Header
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = append(h.method, r.Method)
	h.url = append(h.url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.body = append(h.body, b)
	h.header = append(h.header, r.Header)
	w.Write([]byte(h.content))
}

type testBadHandler struct {
	content string
}

func (h *testBadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, h.content, http.StatusInternalServerError)
}
