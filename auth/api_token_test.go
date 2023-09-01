// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import check "gopkg.in/check.v1"

func (s *S) TestAPIAuth(c *check.C) {
	user := User{Email: "para@xmen.com", APIKey: "Quenço"}
	err := user.Create()
	c.Assert(err, check.IsNil)
	APIKey, err := user.RegenerateAPIKey()
	c.Assert(err, check.IsNil)
	t, err := APIAuth("bearer " + APIKey)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, APIKey)
	c.Assert(t.UserEmail, check.Equals, user.Email)

	c.Assert(user.APIKeyUsageCounter, check.Equals, 0)

	err = user.Reload()
	c.Assert(err, check.IsNil)

	c.Assert(user.APIKeyLastAccess.IsZero(), check.Equals, false)
	c.Assert(user.APIKeyUsageCounter, check.Equals, 1)

}

func (s *S) TestAPIAuthNotFound(c *check.C) {
	t, err := APIAuth("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}
