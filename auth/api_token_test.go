// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestRegenerateAPIToken(c *gocheck.C) {
	user := User{Email: "never@xmen.com"}
	err := s.conn.Users().Insert(&user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	t, err := regenerateAPIToken(&user)
	count, err := s.conn.Users().Find(bson.M{"apikey": t.Token}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
	c.Assert(t.UserEmail, gocheck.Equals, user.Email)
}

func (s *S) TestRegenerateAPITokenReturnsErrorWhenUserReferenceDoesNotContainsEmail(c *gocheck.C) {
	u := User{}
	t, err := regenerateAPIToken(&u)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestRegenerateAPITokenReturnsErrorWhenUserIsNil(c *gocheck.C) {
	t, err := regenerateAPIToken(nil)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User is nil$")
}

func (s *S) TestGetAPIToken(c *gocheck.C) {
	user := User{Email: "para@xmen.com", APIKey: "Quenço"}
	err := s.conn.Users().Insert(&user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	t, err := regenerateAPIToken(&user)
	c.Assert(err, gocheck.IsNil)
	t, err = getAPIToken("bearer " + t.Token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Token, gocheck.Equals, t.Token)
}

func (s *S) TestGetAPITokenEmptyToken(c *gocheck.C) {
	u, err := getAPIToken("bearer tokenthatdoesnotexist")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetAPITokennNotFound(c *gocheck.C) {
	t, err := getAPIToken("bearer invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetAPITokenInvalid(c *gocheck.C) {
	t, err := getAPIToken("invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}
