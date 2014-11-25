// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestGetAPIToken(c *gocheck.C) {
	user := User{Email: "para@xmen.com", APIKey: "Quenço"}
	err := s.conn.Users().Insert(&user)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	APIKey, err := user.RegenerateAPIKey()
	c.Assert(err, gocheck.IsNil)
	t, err := getAPIToken("bearer " + APIKey)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Token, gocheck.Equals, APIKey)
	c.Assert(t.UserEmail, gocheck.Equals, user.Email)
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
