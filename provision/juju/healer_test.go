// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/heal"
	. "launchpad.net/gocheck"
)

func (s *S) TestBootstrapShouldBeRegistered(c *C) {
	h, err := heal.Get("bootstrap")
	c.Assert(err, IsNil)
	c.Assert(h, FitsTypeOf, &BootstrapHealer{})
}

func (s *S) TestBootstrapNeedsHeal(c *C) {
	tmpdir, err := commandmocker.Add("juju", collectOutputBootstrapDown)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	h := BootstrapHealer{}
	c.Assert(h.NeedsHeal(), Equals, true)
}
