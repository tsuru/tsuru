// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	goauth2 "code.google.com/p/goauth2/oauth"
	"github.com/tsuru/tsuru/auth"
	"launchpad.net/gocheck"
)

func (s *S) TestGetToken(c *gocheck.C) {
	existing := Token{Token: goauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, gocheck.IsNil)
	var result []Token
	collection().Find(nil).All(&result)
	t, err := getToken("bearer myvalidtoken")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.AccessToken, gocheck.Equals, "myvalidtoken")
	c.Assert(t.UserEmail, gocheck.Equals, "x@x.com")
}

func (s *S) TestGetTokenEmptyToken(c *gocheck.C) {
	u, err := getToken("bearer tokenthatdoesnotexist")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *gocheck.C) {
	t, err := getToken("bearer invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *gocheck.C) {
	t, err := getToken("invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestSave(c *gocheck.C) {
	existing := Token{Token: goauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, gocheck.IsNil)
	coll := collection()
	defer coll.Close()
	var tokens []Token
	err = coll.Find(nil).All(&tokens)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(tokens), gocheck.Equals, 1)
	c.Assert(tokens[0].GetValue(), gocheck.Equals, "myvalidtoken")
}

func (s *S) TestDelete(c *gocheck.C) {
	existing := Token{Token: goauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save()
	c.Assert(err, gocheck.IsNil)
	err = deleteToken("myvalidtoken")
	c.Assert(err, gocheck.IsNil)
	coll := collection()
	defer coll.Close()
	var tokens []Token
	err = coll.Find(nil).All(&tokens)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(tokens), gocheck.Equals, 0)
}
