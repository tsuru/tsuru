// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	_ "github.com/tsuru/tsuru/router/routertest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestPlanAdd(c *check.C) {
	s.mockService.Plan.OnCreate = func(plan appTypes.Plan) error {
		c.Assert(plan, check.DeepEquals, appTypes.Plan{
			Name:     "xyz",
			Memory:   9223372036854775807,
			Swap:     1024,
			CpuShare: 100,
		})
		return nil
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=1024&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlan, Value: "xyz"},
		Owner:  s.token.GetUserName(),
		Kind:   "plan.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "xyz"},
			{"name": "memory", "value": "9223372036854775807"},
			{"name": "swap", "value": "1024"},
			{"name": "cpushare", "value": "100"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPlanAddWithMegabyteAsMemoryUnit(c *check.C) {
	s.mockService.Plan.OnCreate = func(plan appTypes.Plan) error {
		c.Assert(plan, check.DeepEquals, appTypes.Plan{
			Name:     "xyz",
			Memory:   536870912,
			Swap:     1024,
			CpuShare: 100,
		})
		return nil
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=512M&swap=1024&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
}

func (s *S) TestPlanAddWithMegabyteAsSwapUnit(c *check.C) {
	s.mockService.Plan.OnCreate = func(plan appTypes.Plan) error {
		c.Assert(plan, check.DeepEquals, appTypes.Plan{
			Name:     "xyz",
			Memory:   536870912,
			Swap:     1024,
			CpuShare: 100,
		})
		return nil
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=512M&swap=1024&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
}

func (s *S) TestPlanAddWithGigabyteAsMemoryUnit(c *check.C) {
	s.mockService.Plan.OnCreate = func(plan appTypes.Plan) error {
		c.Assert(plan, check.DeepEquals, appTypes.Plan{
			Name:     "xyz",
			Memory:   9223372036854775807,
			Swap:     536870912,
			CpuShare: 100,
		})
		return nil
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=512M&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
}

func (s *S) TestPlanAddWithNoPermission(c *check.C) {
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=1&swap=2&cpushare=3")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanAddDupp(c *check.C) {
	s.mockService.Plan.OnCreate = func(plan appTypes.Plan) error {
		if plan.CpuShare == 3 {
			return appTypes.ErrPlanAlreadyExists
		}
		c.Assert(plan, check.DeepEquals, appTypes.Plan{
			Name:     "xyz",
			Memory:   9223372036854775807,
			Swap:     1024,
			CpuShare: 100,
		})
		return nil
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=1024&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	body = strings.NewReader("name=xyz&memory=9223372036854775807&swap=2&cpushare=3")
	recorder = httptest.NewRecorder()
	request, err = http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlan, Value: "xyz"},
		Owner:  s.token.GetUserName(),
		Kind:   "plan.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "xyz"},
			{"name": "memory", "value": "9223372036854775807"},
			{"name": "swap", "value": "2"},
			{"name": "cpushare", "value": "3"},
		},
		ErrorMatches: `plan already exists`,
	}, eventtest.HasEvent)
}

func (s *S) TestPlanListEmpty(c *check.C) {
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return nil, nil
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestPlanList(c *check.C) {
	expected := []appTypes.Plan{
		{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3},
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return expected, nil
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var plans []appTypes.Plan
	err = json.Unmarshal(recorder.Body.Bytes(), &plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, expected)
}

func (s *S) TestPlanRemove(c *check.C) {
	recorder := httptest.NewRecorder()
	s.mockService.Plan.OnRemove = func(name string) error {
		c.Assert(name, check.Equals, "plan1")
		return nil
	}
	request, err := http.NewRequest("DELETE", "/plans/plan1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlan, Value: "plan1"},
		Owner:  s.token.GetUserName(),
		Kind:   "plan.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":planname", "value": "plan1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPlanRemoveNoPermission(c *check.C) {
	s.mockService.Plan.OnRemove = func(name string) error {
		c.Error("Plan service not expected to be called.")
		return nil
	}
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/plans/plan1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanRemoveInvalid(c *check.C) {
	s.mockService.Plan.OnRemove = func(name string) error {
		return appTypes.ErrPlanNotFound
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/plans/plan999", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
