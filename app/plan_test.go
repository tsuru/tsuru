// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"launchpad.net/gocheck"
)

func (s *S) TestPlanAdd(c *gocheck.C) {
	p := Plan{
		Name:     "plan1",
		Memory:   9223372036854775807,
		Swap:     1024,
		CpuShare: 100,
	}
	err := p.Save()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, gocheck.IsNil)
	c.Assert(plans, gocheck.DeepEquals, []Plan{p})
}

func (s *S) TestPlanAddInvalid(c *gocheck.C) {
	invalidPlans := []Plan{
		{
			Memory:   9223372036854775807,
			Swap:     1024,
			CpuShare: 100,
		},
		{
			Name:     "plan1",
			Swap:     1024,
			CpuShare: 100,
		},
		{
			Name:     "plan1",
			Memory:   9223372036854775807,
			CpuShare: 100,
		},
		{
			Name:   "plan1",
			Memory: 9223372036854775807,
			Swap:   1024,
		},
	}
	for _, p := range invalidPlans {
		err := p.Save()
		c.Assert(err, gocheck.FitsTypeOf, PlanValidationError{})
	}
}

func (s *S) TestPlanAddDupp(c *gocheck.C) {
	p := Plan{
		Name:     "plan1",
		Memory:   9223372036854775807,
		Swap:     1024,
		CpuShare: 100,
	}
	defer s.conn.Plans().RemoveAll(nil)
	err := p.Save()
	c.Assert(err, gocheck.IsNil)
	err = p.Save()
	c.Assert(err, gocheck.Equals, ErrPlanAlreadyExists)
}

func (s *S) TestPlansList(c *gocheck.C) {
	expected := []Plan{
		{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3},
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	}
	err := s.conn.Plans().Insert(expected[0])
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Plans().Insert(expected[1])
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	plans, err := PlansList()
	c.Assert(err, gocheck.IsNil)
	c.Assert(plans, gocheck.DeepEquals, expected)
}

func (s *S) TestPlanRemove(c *gocheck.C) {
	plans := []Plan{
		{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3},
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	}
	err := s.conn.Plans().Insert(plans[0])
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Plans().Insert(plans[1])
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	err = PlanRemove(plans[0].Name)
	c.Assert(err, gocheck.IsNil)
	var dbPlans []Plan
	err = s.conn.Plans().Find(nil).All(&dbPlans)
	c.Assert(dbPlans, gocheck.DeepEquals, []Plan{
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	})
}

func (s *S) TestPlanRemoveInvalid(c *gocheck.C) {
	err := PlanRemove("xxxx")
	c.Assert(err, gocheck.Equals, ErrPlanNotFound)
}
