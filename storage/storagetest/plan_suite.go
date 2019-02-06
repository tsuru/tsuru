// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type PlanSuite struct {
	SuiteHooks
	PlanStorage app.PlanStorage
}

func sortPlansByName(plans []app.Plan) []app.Plan {
	names := make([]string, len(plans))
	plansMap := make(map[string]app.Plan)
	for i, plan := range plans {
		names[i] = plan.Name
		plansMap[plan.Name] = plan
	}
	sort.Strings(names)
	results := make([]app.Plan, len(plans))
	for i, name := range names {
		results[i] = plansMap[name]
	}
	return results
}

func (s *PlanSuite) TestInsertPlan(c *check.C) {
	p := app.Plan{Name: "myplan", Default: true}
	err := s.PlanStorage.Insert(p)
	c.Assert(err, check.IsNil)
	plan, err := s.PlanStorage.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(plan.Name, check.Equals, p.Name)
	c.Assert(plan.Default, check.Equals, p.Default)
}

func (s *PlanSuite) TestInsertDuplicatePlan(c *check.C) {
	p := app.Plan{Name: "myplan", Default: true}
	err := s.PlanStorage.Insert(p)
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Insert(p)
	c.Assert(err, check.Equals, app.ErrPlanAlreadyExists)
}

func (s *PlanSuite) TestInsertDefaultPlan(c *check.C) {
	p1 := app.Plan{Name: "plan1", Default: true}
	p2 := app.Plan{Name: "plan2", Default: false}
	p3 := app.Plan{Name: "plan3", Default: true}
	err := s.PlanStorage.Insert(p1)
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Insert(p2)
	c.Assert(err, check.IsNil)
	plans, err := s.PlanStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.HasLen, 2)
	sortedPlans := sortPlansByName(plans)
	c.Assert(sortedPlans[0].Name, check.Equals, p1.Name)
	c.Assert(sortedPlans[0].Default, check.Equals, true)
	c.Assert(sortedPlans[1].Name, check.Equals, p2.Name)
	c.Assert(sortedPlans[1].Default, check.Equals, false)
	err = s.PlanStorage.Insert(p3)
	c.Assert(err, check.IsNil)
	plans, err = s.PlanStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.HasLen, 3)
	sortedPlans = sortPlansByName(plans)
	c.Assert(sortedPlans[0].Name, check.Equals, p1.Name)
	c.Assert(sortedPlans[0].Default, check.Equals, false)
	c.Assert(sortedPlans[1].Name, check.Equals, p2.Name)
	c.Assert(sortedPlans[1].Default, check.Equals, false)
	c.Assert(sortedPlans[2].Name, check.Equals, p3.Name)
	c.Assert(sortedPlans[2].Default, check.Equals, true)
}

func (s *PlanSuite) TestFindAllPlans(c *check.C) {
	err := s.PlanStorage.Insert(app.Plan{Name: "plan1"})
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Insert(app.Plan{Name: "plan2", Default: true})
	c.Assert(err, check.IsNil)
	plans, err := s.PlanStorage.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.HasLen, 2)
	names := []string{plans[0].Name, plans[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"plan1", "plan2"})
}

func (s *PlanSuite) TestFindDefaultPlan(c *check.C) {
	err := s.PlanStorage.Insert(app.Plan{Name: "plan1"})
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Insert(app.Plan{Name: "plan2", Default: true})
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Insert(app.Plan{Name: "plan3", Default: false})
	c.Assert(err, check.IsNil)
	plan, err := s.PlanStorage.FindDefault()
	c.Assert(err, check.IsNil)
	c.Assert(plan, check.NotNil)
	c.Assert(plan.Name, check.Equals, "plan2")
}

func (s *PlanSuite) TestFindDefaultPlanNotFound(c *check.C) {
	err := s.PlanStorage.Insert(app.Plan{Name: "plan1", Default: false})
	c.Assert(err, check.IsNil)
	plan, err := s.PlanStorage.FindDefault()
	c.Assert(err, check.Equals, app.ErrPlanDefaultNotFound)
	c.Assert(plan, check.IsNil)
}

func (s *PlanSuite) TestFindPlanByName(c *check.C) {
	p := app.Plan{Name: "myteam"}
	err := s.PlanStorage.Insert(p)
	c.Assert(err, check.IsNil)
	plan, err := s.PlanStorage.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(plan.Name, check.Equals, p.Name)
}

func (s *PlanSuite) TestFindPlanByNameNotFound(c *check.C) {
	plan, err := s.PlanStorage.FindByName("wat")
	c.Assert(err, check.Equals, app.ErrPlanNotFound)
	c.Assert(plan, check.IsNil)
}

func (s *PlanSuite) TestDeletePlan(c *check.C) {
	plan := app.Plan{Name: "myplan"}
	err := s.PlanStorage.Insert(plan)
	c.Assert(err, check.IsNil)
	err = s.PlanStorage.Delete(plan)
	c.Assert(err, check.IsNil)
	p, err := s.PlanStorage.FindByName("myplan")
	c.Assert(err, check.Equals, app.ErrPlanNotFound)
	c.Assert(p, check.IsNil)
}

func (s *PlanSuite) TestDeletePlanNotFound(c *check.C) {
	err := s.PlanStorage.Delete(app.Plan{Name: "myteam"})
	c.Assert(err, check.Equals, app.ErrPlanNotFound)
}
