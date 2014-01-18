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
	defer DeletePlan(&plan)
	p, err := GetPlanByName(plan.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Name, gocheck.Equals, plan.Name)
	c.Assert(p.ServiceName, gocheck.Equals, plan.ServiceName)
}

func (s *S) TestRemovePlan(c *gocheck.C) {
	plan := Plan{
		Name:        "Ignite",
		ServiceName: "MySql",
	}
	err := CreatePlan(&plan)
	c.Assert(err, gocheck.IsNil)
	err = DeletePlan(&plan)
	c.Assert(err, gocheck.IsNil)
	p, err := GetPlanByName("Ignite")
	c.Assert(err, gocheck.NotNil)
	c.Assert(p, gocheck.IsNil)
}
