// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"sync"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestPlanAdd(c *check.C) {
	p := appTypes.Plan{
		Name:   "plan1",
		Memory: 9223372036854775807,
	}
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnInsert: func(plan appTypes.Plan) error {
				c.Assert(p, check.Equals, plan)
				return nil
			},
		},
	}
	err := ps.Create(context.TODO(), p)
	c.Assert(err, check.IsNil)
}

func (s *S) TestPlanAddInvalid(c *check.C) {
	invalidPlans := []appTypes.Plan{
		{
			Memory: 9223372036854775807,
		},
		{
			Name:   "plan1",
			Memory: 4,
		},
	}
	expectedError := []error{appTypes.PlanValidationError{Field: "name"}, appTypes.ErrLimitOfMemory}
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnInsert: func(appTypes.Plan) error {
				c.Error("storage.Insert should not be called")
				return nil
			},
		},
	}
	for i, p := range invalidPlans {
		err := ps.Create(context.TODO(), p)
		c.Assert(err, check.FitsTypeOf, expectedError[i])
	}
}

func (s *S) TestPlansList(c *check.C) {
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnFindAll: func() ([]appTypes.Plan, error) {
				return []appTypes.Plan{
					{Name: "plan1", Memory: 1},
					{Name: "plan2", Memory: 3},
				}, nil
			},
		},
	}
	plans, err := ps.List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.HasLen, 2)
}

func (s *S) TestPlanRemove(c *check.C) {
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnDelete: func(plan appTypes.Plan) error {
				c.Assert(plan.Name, check.Equals, "Plan1")
				return nil
			},
		},
	}
	err := ps.Remove(context.TODO(), "Plan1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestDefaultPlan(c *check.C) {
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnFindDefault: func() (*appTypes.Plan, error) {
				return &appTypes.Plan{
					Name:    "default-plan",
					Memory:  1024,
					Default: true,
				}, nil
			},
		},
	}
	p, err := ps.DefaultPlan(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestDefaultPlanAutoGenerated(c *check.C) {
	autogenerated := appTypes.Plan{
		Name:     "c0.1m0.2",
		Memory:   268435456,
		CPUMilli: 100,
		Default:  true,
	}
	ps, err := PlanService()
	c.Assert(err, check.IsNil)
	p, err := ps.DefaultPlan(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(p, check.DeepEquals, &autogenerated)
}

func (s *S) TestDefaultPlanNotFoundPlansNotEmpty(c *check.C) {
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnFindDefault: func() (*appTypes.Plan, error) {
				return nil, appTypes.ErrPlanDefaultNotFound
			},
			OnFindAll: func() ([]appTypes.Plan, error) {
				return []appTypes.Plan{{Name: "p1"}}, nil
			},
			OnInsert: func(plan appTypes.Plan) error {
				c.Error("storage.Insert should not be called")
				return nil
			},
		},
	}
	_, err := ps.DefaultPlan(context.TODO())
	c.Assert(err, check.Equals, appTypes.ErrPlanDefaultNotFound)
}

func (s *S) TestFindPlanByName(c *check.C) {
	ps := &planService{
		storage: &appTypes.MockPlanStorage{
			OnFindByName: func(name string) (*appTypes.Plan, error) {
				c.Check(name, check.Equals, "plan1")
				return &appTypes.Plan{
					Name:   "plan1",
					Memory: 9223372036854775807,
				}, nil
			},
		},
	}
	plan, err := ps.FindByName(context.TODO(), "plan1")
	c.Assert(err, check.IsNil)
	c.Assert(plan.Name, check.Equals, "plan1")
}

func (s *S) TestDefaultPlanAutoGeneratedRace(c *check.C) {
	autogenerated := appTypes.Plan{
		Name:     "c0.1m0.2",
		Memory:   268435456,
		CPUMilli: 100,
		Default:  true,
	}
	ps, err := PlanService()
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	nGoroutines := 10
	for i := 0; i < nGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, pErr := ps.DefaultPlan(context.TODO())
			c.Check(pErr, check.IsNil)
			c.Check(p, check.DeepEquals, &autogenerated)
		}()
	}
	wg.Wait()
	plans, err := ps.List(context.TODO())
	c.Assert(err, check.IsNil)
	c.Check(plans, check.HasLen, len(defaultPlans))
}
