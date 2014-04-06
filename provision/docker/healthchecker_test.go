// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

type HealthSuite struct{}

var _ = gocheck.Suite(&HealthSuite{})

func (s *HealthSuite) TestIsUnreachable(c *gocheck.C) {
	app := testing.NewFakeApp("almah", "static", 1)
	units := app.ProvisionedUnits()
	reachable, err := IsReachable(units[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(reachable, gocheck.Equals, false)
}
