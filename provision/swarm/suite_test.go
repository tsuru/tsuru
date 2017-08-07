// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	fakebuilder "github.com/tsuru/tsuru/builder/fake"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type S struct {
	p     *swarmProvisioner
	b     *fakebuilder.FakeBuilder
	conn  *db.Storage
	user  *auth.User
	team  *auth.Team
	token auth.Token
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_swarm_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	config.Set("docker:registry", "registry.tsuru.io")
	config.Set("host", "http://tsuruhost")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	rand.Seed(0)
	config.Set("swarm:swarm-port", 0)
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	err = provision.AddPool(provision.AddPoolOptions{Name: "bonehunters", Default: true, Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	p := app.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	err = p.Save()
	c.Assert(err, check.IsNil)
	s.p = &swarmProvisioner{}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	s.b = &fakebuilder.FakeBuilder{}
	s.user = &auth.User{Email: "whiskeyjack@genabackis.com", Password: "123456", Quota: quota.Unlimited}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{Name: "admin"}
	c.Assert(err, check.IsNil)
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *S) addServiceInstance(c *check.C, appName string, fn http.HandlerFunc) func() {
	ts := httptest.NewServer(fn)
	ret := func() {
		ts.Close()
		s.conn.Services().Remove(bson.M{"_id": "mysql"})
		s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	}
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{}, Apps: []string{appName}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	return ret
}
