// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package saml

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	check "gopkg.in/check.v1"
)

func (s *S) TestGetToken(c *check.C) {
	user := &auth.User{Email: "x@x.com"}
	token, err := createToken(user)
	c.Assert(err, check.IsNil)
	count, err := s.conn.Tokens().Find(bson.M{"useremail": "x@x.com"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
	t, err := getToken("bearer " + token.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, token.Token)
	c.Assert(t.UserEmail, check.Equals, "x@x.com")
}

func (s *S) TestGetTokenEmptyToken(c *check.C) {
	u, err := getToken("bearer tokenthatdoesnotexist")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *check.C) {
	t, err := getToken("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *check.C) {
	t, err := getToken("invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}
