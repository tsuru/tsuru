// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"context"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"golang.org/x/oauth2"
	check "gopkg.in/check.v1"
)

func (s *S) TestGetToken(c *check.C) {
	existing := tokenWrapper{Token: oauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save(context.TODO())
	c.Assert(err, check.IsNil)
	var result []tokenWrapper

	collection, err := storagev2.OAuth2TokensCollection()
	c.Assert(err, check.IsNil)

	cursor, err := collection.Find(context.TODO(), mongoBSON.M{})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &result)
	c.Assert(err, check.IsNil)

	t, err := getToken(context.TODO(), "bearer myvalidtoken")
	c.Assert(err, check.IsNil)
	c.Assert(t.AccessToken, check.Equals, "myvalidtoken")
	c.Assert(t.UserEmail, check.Equals, "x@x.com")
}

func (s *S) TestGetTokenEmptyToken(c *check.C) {
	u, err := getToken(context.TODO(), "bearer tokenthatdoesnotexist")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *check.C) {
	t, err := getToken(context.TODO(), "bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *check.C) {
	t, err := getToken(context.TODO(), "invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestSave(c *check.C) {
	existing := tokenWrapper{Token: oauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save(context.TODO())
	c.Assert(err, check.IsNil)
	collection, err := storagev2.OAuth2TokensCollection()
	c.Assert(err, check.IsNil)
	var tokens []tokenWrapper
	cursor, err := collection.Find(context.TODO(), mongoBSON.M{})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &tokens)
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 1)
	c.Assert(tokens[0].GetValue(), check.Equals, "myvalidtoken")
}

func (s *S) TestDelete(c *check.C) {
	existing := tokenWrapper{Token: oauth2.Token{AccessToken: "myvalidtoken"}, UserEmail: "x@x.com"}
	err := existing.save(context.TODO())
	c.Assert(err, check.IsNil)
	err = deleteToken(context.TODO(), "myvalidtoken")
	c.Assert(err, check.IsNil)
	collection, err := storagev2.OAuth2TokensCollection()
	c.Assert(err, check.IsNil)

	var tokens []tokenWrapper
	cursor, err := collection.Find(context.TODO(), mongoBSON.M{})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &tokens)
	c.Assert(err, check.IsNil)

	c.Assert(tokens, check.HasLen, 0)
}
