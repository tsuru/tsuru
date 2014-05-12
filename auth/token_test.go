// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"launchpad.net/gocheck"
)

func (s *S) TestParseToken(c *gocheck.C) {
	t, err := ParseToken("type token")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.Equals, "token")
	t, err = ParseToken("token")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.Equals, "token")
	t, err = ParseToken("type ble ble")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
	c.Assert(t, gocheck.Equals, "")
	t, err = ParseToken("")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
	c.Assert(t, gocheck.Equals, "")
}
