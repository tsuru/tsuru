// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/tsuru/tsuru/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestGetToken(c *check.C) {
	existing := Token{Token: oauth.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, check.IsNil)
	var result []Token
	collection().Find(nil).All(&result)
	t, err := getToken("bearer myvalidtoken")
	c.Assert(err, check.IsNil)
	c.Assert(t.AccessToken, check.Equals, "myvalidtoken")
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

func (s *S) TestSave(c *check.C) {
	existing := Token{Token: oauth.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, check.IsNil)
	coll := collection()
	defer coll.Close()
	var tokens []Token
	err = coll.Find(nil).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokens), check.Equals, 1)
	c.Assert(tokens[0].GetValue(), check.Equals, "myvalidtoken")
}

func (s *S) TestDelete(c *check.C) {
	existing := Token{Token: oauth.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, check.IsNil)
	err = deleteToken("myvalidtoken")
	c.Assert(err, check.IsNil)
	coll := collection()
	defer coll.Close()
	var tokens []Token
	err = coll.Find(nil).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokens), check.Equals, 0)
}
