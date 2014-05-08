// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"github.com/tsuru/tsuru/db"
	"launchpad.net/gocheck"
)

func (s *S) TestNewToken(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	t, err := newToken("somecode")
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(t)
}
