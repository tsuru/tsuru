// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"launchpad.net/gocheck"
	"os"
)

func (s *S) TestClientID(c *gocheck.C) {
	err := os.Setenv("TSURU_AUTH_CLIENTID", "someid")
	c.Assert(err, gocheck.IsNil)
	c.Assert("someid", gocheck.Equals, clientID())
}
