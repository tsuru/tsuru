// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestAppTokenAuth(c *check.C) {
	appToken := authTypes.AppToken{}
	err := AppTokenService().Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := AppTokenAuth(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.GetValue(), check.Equals, appToken.Token)
}

func (s *S) TestAppTokenAuthNotFound(c *check.C) {
	t, err := AppTokenAuth("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}
