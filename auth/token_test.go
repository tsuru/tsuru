// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/config"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestNewTokenIsStoredInUser(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, "wolverine@xmen.com")
	c.Assert(u.Tokens[0].Token, gocheck.Equals, t.Token)
}

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
