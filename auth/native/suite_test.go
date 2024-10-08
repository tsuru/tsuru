// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db/storagev2"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	user   *auth.User
	team   *authTypes.Team
	server *authtest.SMTPServer
	token  auth.Token
}

var _ = check.Suite(&S{})

var nativeScheme = NativeScheme{}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("auth:token-expire-days", 2)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_auth_native_test")

	storagev2.Reset()

	var err error
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
}

func (s *S) SetUpTest(c *check.C) {
	s.user = &auth.User{Email: "timeredbull@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), s.user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "cobrateam"}
}

func (s *S) TearDownTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	cost = 0
	tokenExpire = 0
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
	storagev2.ClearAllCollections(nil)
}
