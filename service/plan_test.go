// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"launchpad.net/gocheck"
)

func (s *S) TestCreatePlan(c *gocheck.C) {
	plan := Plan{
		Name:        "Ignite",
		ServiceName: "MySql",
	}
	err := CreatePlan(&plan)
	c.Assert(err, gocheck.IsNil)
	p, err := GetPlan(plan.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Name, gocheck.Equals, plan.Name)
	c.Assert(p.ServiceName, gocheck.Equals, plan.ServiceName)
}
