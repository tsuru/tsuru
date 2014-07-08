// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas/cloudstack"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestGetIaaS(c *gocheck.C) {
	config.Set("iaas:provider", "test")
	_, err := GetIaaS()
	c.Assert(err, gocheck.ErrorMatches, "IaaS provider \"test\" not registered")
	config.Set("iaas:provider", "cloudstack")
	iaas, err := GetIaaS()
	c.Assert(err, gocheck.IsNil)
	_, ok := iaas.(*cloudstack.CloudstackIaaS)
	c.Assert(ok, gocheck.Equals, true)
}
