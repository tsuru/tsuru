// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package peer

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"gopkg.in/check.v1"
)

func TestT(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestAuthFailsClosedWhenTokenUnset(c *check.C) {
	config.Unset("auth:peer:token")
	c.Assert(TokenValue(), check.Equals, "")
	_, err := Auth(context.Background(), "97ef27b972cb168a9875a32cba96010e1eb6a6065e9f4cc5928a7cb66d2a6b86")
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestAuthWithConfiguredToken(c *check.C) {
	defer config.Unset("auth:peer:token")
	config.Set("auth:peer:token", "my-peer-secret")
	token, err := Auth(context.Background(), "my-peer-secret")
	c.Assert(err, check.IsNil)
	c.Assert(token.GetUserName(), check.Equals, "peer")
	_, err = Auth(context.Background(), "my-peer-secret-other")
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}
