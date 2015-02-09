// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/gorilla/context"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	_ "github.com/tsuru/tsuru/queue/queuetest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
)

type testHandler struct {
	body    [][]byte
	method  []string
	url     []string
	content string
	header  []http.Header
	rspCode int
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = append(h.method, r.Method)
	h.url = append(h.url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.body = append(h.body, b)
	h.header = append(h.header, r.Header)
	if h.rspCode == 0 {
		h.rspCode = http.StatusOK
	}
	w.WriteHeader(h.rspCode)
	w.Write([]byte(h.content))
}

type testBadHandler struct{}

func (h *testBadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "some error", http.StatusInternalServerError)
}

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	team        *auth.Team
	user        *auth.User
	token       auth.Token
	adminteam   *auth.Team
	adminuser   *auth.User
	admintoken  auth.Token
	provisioner *provisiontest.FakeProvisioner
}

var _ = check.Suite(&S{})

type hasAccessToChecker struct{}

func (c *hasAccessToChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "HasAccessTo", Params: []string{"team", "service"}}
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

var HasAccessTo check.Checker = &hasAccessToChecker{}

func (s *S) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456", Quota: quota.Unlimited}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.adminuser = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.Unlimited}
	_, err = nativeScheme.Create(s.adminuser)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.adminteam = &auth.Team{Name: "admin", Users: []string{s.adminuser.Email}}
	err = s.conn.Teams().Insert(s.adminteam)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	s.admintoken, err = nativeScheme.Login(map[string]string{"email": s.adminuser.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

var nativeScheme = auth.ManagedScheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.provisioner = provisiontest.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	app.AuthScheme = nativeScheme
	p := app.Platform{Name: "zend"}
	s.conn.Platforms().Insert(p)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *S) TearDownTest(c *check.C) {
	s.provisioner.Reset()
	context.Purge(-1)
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
