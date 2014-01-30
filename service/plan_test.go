// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"launchpad.net/gocheck"
)

func (s *S) TestPlan(c *gocheck.C) {
	plan := Plan{
		Name:        "Ignite",
		Description: "A simple plan",
	}
	c.Assert("Ignite", gocheck.Equals, plan.Name)
	c.Assert("A simple plan", gocheck.Equals, plan.Description)
}
