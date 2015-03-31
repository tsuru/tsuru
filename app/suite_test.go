// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	_ "github.com/tsuru/tsuru/queue/queuetest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	team        auth.Team
	user        *auth.User
	adminTeam   auth.Team
	admin       *auth.User
	provisioner *provisiontest.FakeProvisioner
	defaultPlan Plan
}

var _ = check.Suite(&S{})

type greaterChecker struct{}

func (c *greaterChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "Greater", Params: []string{"expected", "obtained"}}
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

var Greater check.Checker = &greaterChecker{}

func (s *S) createUserAndTeam(c *check.C) {
	s.user = &auth.User{
		Email: "whydidifall@thewho.com",
		Quota: quota.Unlimited,
	}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
}

var nativeScheme = auth.Scheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.createUserAndTeam(c)
	s.provisioner = provisiontest.NewFakeProvisioner()
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
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *S) SetUpTest(c *check.C) {
	repository.Manager().CreateUser(s.user.Email)
	factory, err := queue.Factory()
	c.Assert(err, check.IsNil)
	factory.Reset()
}

func (s *S) TearDownTest(c *check.C) {
	repositorytest.Reset()
	s.provisioner.Reset()
	LogRemove(nil)
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota": quota.Unlimited}},
	)
	s.conn.AutoScale().RemoveAll(nil)
	s.conn.Deploys().RemoveAll(nil)
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

func (s *S) createAdminUserAndTeam(c *check.C) {
	s.admin = &auth.User{Email: "superuser@gmail.com"}
	err := s.admin.Create()
	c.Assert(err, check.IsNil)
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, check.IsNil)
	s.adminTeam = auth.Team{Name: adminTeamName, Users: []string{s.admin.Email}}
	err = s.conn.Teams().Insert(&s.adminTeam)
	c.Assert(err, check.IsNil)
}

func (s *S) removeAdminUserAndTeam(c *check.C) {
	err := s.conn.Teams().RemoveId(s.adminTeam.Name)
	c.Assert(err, check.IsNil)
	err = s.conn.Users().Remove(bson.M{"email": s.admin.Email})
	c.Assert(err, check.IsNil)
}

func (s *S) addServiceInstance(c *check.C, appName string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
		s.conn.Services().Remove(bson.M{"_id": "mysql"})
		s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	err = instance.AddApp(appName)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Update(bson.M{"name": instance.Name}, instance)
	c.Assert(err, check.IsNil)
	return ret
}
