// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/service"
	tsuruTesting "github.com/tsuru/tsuru/testing"
	"io"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"os"
	"path"
	"testing"
)

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

type testBadHandler struct{}

func (h *testBadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "some error", http.StatusInternalServerError)
}

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn        *db.Storage
	team        *auth.Team
	user        *auth.User
	token       auth.Token
	adminteam   *auth.Team
	adminuser   *auth.User
	admintoken  auth.Token
	t           *tsuruTesting.T
	provisioner *tsuruTesting.FakeProvisioner
}

var _ = gocheck.Suite(&S{})

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
}

func (c *hasAccessToChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you must provide two parameters"
	}
	team, ok := params[0].(auth.Team)
	if !ok {
		return false, "first parameter should be a team instance"
	}
	srv, ok := params[1].(service.Service)
	if !ok {
		return false, "second parameter should be service instance"
	}
	return srv.HasTeam(&team), ""
}

var HasAccessTo gocheck.Checker = &hasAccessToChecker{}

func (s *S) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456", Quota: quota.Unlimited}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, gocheck.IsNil)
	s.adminuser = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(s.adminuser)
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.adminteam = &auth.Team{Name: "admin", Users: []string{s.adminuser.Email}}
	err = s.conn.Teams().Insert(s.adminteam)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	s.admintoken, err = nativeScheme.Login(map[string]string{"email": s.adminuser.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *gocheck.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.createUserAndTeam(c)
	s.t = &tsuruTesting.T{}
	s.provisioner = tsuruTesting.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	app.AuthScheme = nativeScheme
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	queue.Preempt()
	s.conn.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.t.RollbackGitConfs(c)
	s.provisioner.Reset()
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

func resetHandlers() {
	tsuruHandlerList = []TsuruHandler{}
}
