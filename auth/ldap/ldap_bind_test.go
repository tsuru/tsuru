// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ldap

import (
	"runtime"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"gopkg.in/check.v1"
)

func (s *S) TestCreateTokenShouldSaveTheTokenInTheDatabase(c *check.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	_, err = createToken(&u, "123456")
	c.Assert(err, check.IsNil)
	var result native.Token
	err = s.conn.Tokens().Find(bson.M{"useremail": u.Email}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Token, check.NotNil)
}

func (s *S) TestCreateTokenRemoveOldTokens(c *check.C) {
	config.Set("auth:max-simultaneous-sessions", 2)
	u := auth.User{Email: "para@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": u.Email})
	t1, err := native.NewUserToken(&u)
	c.Assert(err, check.IsNil)
	t2 := t1
	t2.Token += "aa"
	err = s.conn.Tokens().Insert(t1, t2)
	c.Assert(err, check.IsNil)
	_, err = createToken(&u, "123456")
	c.Assert(err, check.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			ct, err := s.conn.Tokens().Find(bson.M{"useremail": u.Email}).Count()
			c.Assert(err, check.IsNil)
			if ct == 2 {
				ok <- true
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Fatal("Did not remove old tokens after 2 seconds")
	}
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveEmailDefined(c *check.C) {
	u := auth.User{Password: "123"}
	_, err := createToken(&u, "123")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^User does not have an email$")
}

func (s *S) TestCreateTokenShouldValidateThePassword(c *check.C) {
	u := auth.User{Email: "me@gmail.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	_, err = createToken(&u, "123")
	c.Assert(err, check.NotNil)
}

func (s *S) TestTokenGetUser(c *check.C) {
	u, err := s.token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, s.user.Email)
}
