// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "gopkg.in/check.v1"

func (s *S) TestGetAPIToken(c *check.C) {
	user := User{Email: "para@xmen.com", APIKey: "Quenço"}
	err := user.Create()
	c.Assert(err, check.IsNil)
	defer user.Delete()
	APIKey, err := user.RegenerateAPIKey()
	c.Assert(err, check.IsNil)
	t, err := getAPIToken("bearer " + APIKey)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, APIKey)
	c.Assert(t.UserEmail, check.Equals, user.Email)
}

func (s *S) TestGetAPITokenEmptyToken(c *check.C) {
	u, err := getAPIToken("bearer tokenthatdoesnotexist")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}

func (s *S) TestGetAPITokennNotFound(c *check.C) {
	t, err := getAPIToken("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}

func (s *S) TestGetAPITokenInvalid(c *check.C) {
	t, err := getAPIToken("invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}
