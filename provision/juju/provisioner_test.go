// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/provision"
	. "launchpad.net/gocheck"
)

func (s *S) TestShouldBeRegistered(c *C) {
	p, err := provision.Get("juju")
	c.Assert(err, IsNil)
	c.Assert(p, FitsTypeOf, &JujuProvisioner{})
}
