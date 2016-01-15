// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
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
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn        *db.Storage
	logConn     *db.LogStorage
	team        auth.Team
	user        *auth.User
	provisioner *provisiontest.FakeProvisioner
	defaultPlan Plan
	Pool        string
	zeroLock    map[string]interface{}
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

func customUserWithPermission(c *check.C, baseName string, perm ...permission.Permission) *auth.User {
	user := &auth.User{Email: baseName + "@groundcontrol.com", Password: "123456", Quota: quota.Unlimited}
	_, err := nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	for _, p := range perm {
		role, err := permission.NewRole(baseName+p.Scheme.FullName()+p.Context.Value, string(p.Context.CtxType))
		c.Assert(err, check.IsNil)
		name := p.Scheme.FullName()
		if name == "" {
			name = "*"
		}
		err = role.AddPermissions(name)
		c.Assert(err, check.IsNil)
		err = user.AddRole(role.Name, p.Context.Value)
		c.Assert(err, check.IsNil)
	}
	return user
}

func (s *S) createUserAndTeam(c *check.C) {
	s.user = &auth.User{
		Email: "whydidifall@thewho.com",
		Quota: quota.Unlimited,
	}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = auth.Team{Name: "tsuruteam"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
}

var nativeScheme = auth.Scheme(native.NativeScheme{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("testdata/config.yaml")
	c.Assert(err, check.IsNil)
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.logConn, err = db.LogConn()
	c.Assert(err, check.IsNil)
	s.provisioner = provisiontest.NewFakeProvisioner()
	Provisioner = s.provisioner
	AuthScheme = nativeScheme
	data, err := json.Marshal(AppLock{})
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(data, &s.zeroLock)
	c.Assert(err, check.IsNil)
	LogPubSubQueuePrefix = "pubsub:app-test:"
}

func (s *S) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	defer s.logConn.Close()
	s.conn.Apps().Database.DropDatabase()
	s.logConn.Logs("myapp").Database.DropDatabase()
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	routertest.HCRouter.Reset()
	s.provisioner.Reset()
	repositorytest.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUserAndTeam(c)
	platform := Platform{Name: "python"}
	s.conn.Platforms().Insert(platform)
	s.defaultPlan = Plan{
		Name:     "default-plan",
		Memory:   1024,
		Swap:     1024,
		CpuShare: 100,
		Default:  true,
	}
	err := s.conn.Plans().Insert(s.defaultPlan)
	c.Assert(err, check.IsNil)
	s.Pool = "pool1"
	opts := provision.AddPoolOptions{Name: s.Pool, Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(s.user.Email)
	factory, err := queue.Factory()
	c.Assert(err, check.IsNil)
	factory.Reset()
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
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
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}, Apps: []string{appName}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	return ret
}
