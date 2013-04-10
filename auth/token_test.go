// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"time"
)

func (s *S) TestNewTokenReturnsErroWhenUserReferenceDoesNotContainsEmail(c *gocheck.C) {
	u := User{}
	t, err := newUserToken(&u)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *gocheck.C) {
	t, err := newUserToken(nil)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User is nil$")
}

func (s *S) TestNewTokenWithoutTokenKey(c *gocheck.C) {
	old, err := config.Get("auth:token-key")
	c.Assert(err, gocheck.IsNil)
	defer config.Set("auth:token-key", old)
	err = config.Unset("auth:token-key")
	c.Assert(err, gocheck.IsNil)
	t, err := newUserToken(&User{Email: "gopher@golang.org"})
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "auth:token-key" is undefined.`)
}

func (s *S) TestCheckTokenReturnErrorIfTheTokenIsOmited(c *gocheck.C) {
	u, err := CheckToken("")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^You must provide the token$")
}

func (s *S) TestCheckTokenReturnErrorIfTheTokenIsInvalid(c *gocheck.C) {
	u, err := CheckToken("invalid")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Invalid token$")
}

func (s *S) TestCheckTokenReturnTheUserIfTheTokenIsValid(c *gocheck.C) {
	u, e := CheckToken(s.token.Token)
	c.Assert(e, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, s.user.Email)
}

func (s *S) TestCreateApplicationToken(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.NotNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	n, err := s.conn.Tokens().Find(bson.M{"token": t.Token}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(t.AppName, gocheck.Equals, "tsuru-healer")
}

func (s *S) TestCheckApplicationToken(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t2, err := CheckApplicationToken(t.Token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t2.Token, gocheck.Equals, t.Token)
}

func (s *S) TestCheckApplicationTokenExpired(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t.ValidUntil = time.Now().Add(-24 * time.Hour)
	s.conn.Tokens().Update(bson.M{"token": t.Token}, t)
	t2, err := CheckApplicationToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid token.")
}

func (s *S) TestCheckApplicationTokenUnknown(c *gocheck.C) {
	t, err := CheckApplicationToken("unknown")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid token.")
}

func (s *S) TestCheckApplicationTokenUserToken(c *gocheck.C) {
	t := Token{
		UserEmail:  "something@something.com",
		Token:      "12ghoasojad",
		ValidUntil: time.Now().Add(time.Hour),
	}
	s.conn.Tokens().Insert(t)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t2, err := CheckApplicationToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid token.")
}

func (s *S) TestTokenMarshalJSON(c *gocheck.C) {
	valid := time.Now()
	t := Token{
		Token:      "12saii",
		ValidUntil: valid,
		UserEmail:  "something@something.com",
		AppName:    "myapp",
	}
	b, err := json.Marshal(&t)
	c.Assert(err, gocheck.IsNil)
	want := fmt.Sprintf(`{"token":"12saii","valid-until":"%s","email":"something@something.com","app":"myapp"}`,
		valid.Format(time.RFC3339Nano))
	c.Assert(string(b), gocheck.Equals, want)
}
