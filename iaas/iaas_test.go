// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"launchpad.net/gocheck"
)

func (s *S) TestRegisterIaasProvider(c *gocheck.C) {
	provider, err := getIaasProvider("abc")
	c.Assert(err, gocheck.ErrorMatches, "IaaS provider \"abc\" not registered")
	c.Assert(provider, gocheck.IsNil)
	providerInstance := TestIaaS{}
	RegisterIaasProvider("abc", providerInstance)
	provider, err = getIaasProvider("abc")
	c.Assert(err, gocheck.IsNil)
	c.Assert(provider, gocheck.Equals, providerInstance)
}
