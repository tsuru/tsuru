// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"math/rand"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/router/routertest"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/check.v1"
)

type S struct {
	p     *mesosProvisioner
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
	config.Set("kubernetes:token", "token==")
	config.Set("database:name", "provision_mesos_tests_s")
	config.Set("routers:fake:type", "fake")
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
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	err = provision.AddPool(provision.AddPoolOptions{
		Name:        "bonehunters",
		Default:     true,
		Provisioner: "mesos",
	})
	c.Assert(err, check.IsNil)
	p := app.Plan{
		Name:     "default",
		Router:   "fake",
		Default:  true,
		CpuShare: 100,
	}
	err = p.Save()
	c.Assert(err, check.IsNil)
	s.p = &mesosProvisioner{}
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
