// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/service"
	ttesting "github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn        *db.Storage
	team        auth.Team
	user        *auth.User
	adminTeam   auth.Team
	admin       *auth.User
	t           *ttesting.T
	provisioner *ttesting.FakeProvisioner
	defaultPlan Plan
}

var _ = gocheck.Suite(&S{})

type greaterChecker struct{}

func (c *greaterChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "Greater", Params: []string{"expected", "obtained"}}
}

func (c *greaterChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should pass two values to compare"
	}
	n1, ok := params[0].(int)
	if !ok {
		return false, "first parameter should be int"
	}
	n2, ok := params[1].(int)
	if !ok {
		return false, "second parameter should be int"
	}
	if n1 > n2 {
		return true, ""
	}
	err := fmt.Sprintf("%d is not greater than %d", params[0], params[1])
	return false, err
}

var Greater gocheck.Checker = &greaterChecker{}

func (s *S) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{
		Email: "whydidifall@thewho.com",
		Quota: quota.Unlimited,
	}
	err := s.user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
}

var nativeScheme = auth.Scheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *gocheck.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, gocheck.IsNil)
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.t = &ttesting.T{}
	s.createUserAndTeam(c)
	s.t.SetGitConfs(c)
	s.provisioner = ttesting.NewFakeProvisioner()
	Provisioner = s.provisioner
	AuthScheme = nativeScheme
	platform := Platform{Name: "python"}
	s.conn.Platforms().Insert(platform)
	s.defaultPlan = Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	err = s.conn.Plans().Insert(s.defaultPlan)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.t.RollbackGitConfs(c)
	s.provisioner.Reset()
	LogRemove(nil)
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota": quota.Unlimited}},
	)
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

func (s *S) createAdminUserAndTeam(c *gocheck.C) {
	s.admin = &auth.User{Email: "superuser@gmail.com"}
	err := s.admin.Create()
	c.Assert(err, gocheck.IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, gocheck.IsNil)
	s.adminTeam = auth.Team{Name: adminTeamName, Users: []string{s.admin.Email}}
	err = s.conn.Teams().Insert(&s.adminTeam)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) removeAdminUserAndTeam(c *gocheck.C) {
	err := s.conn.Teams().RemoveId(s.adminTeam.Name)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Users().Remove(bson.M{"email": s.admin.Email})
	c.Assert(err, gocheck.IsNil)
}

func (s *S) addServiceInstance(c *gocheck.C, appName string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
		s.conn.Services().Remove(bson.M{"_id": "mysql"})
		s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	err = instance.AddApp(appName)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Update(bson.M{"name": instance.Name}, instance)
	c.Assert(err, gocheck.IsNil)
	return ret
}

type testHandler struct {
	body    [][]byte
	method  []string
	url     []string
	content string
	header  []http.Header
	request *http.Request
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = append(h.method, r.Method)
	h.url = append(h.url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.body = append(h.body, b)
	h.header = append(h.header, r.Header)
	h.request = r
	w.Write([]byte(h.content))
}

type testBadHandler struct {
	msg string
}

func (h *testBadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, h.msg, http.StatusInternalServerError)
}
