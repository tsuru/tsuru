// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *S) TestPlanAdd(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=1024&cpushare=100&router=fake")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "xyz", Memory: 9223372036854775807, Swap: 1024, CpuShare: 100, Router: "fake"},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePlan, Value: "xyz"},
		Owner:  s.token.GetUserName(),
		Kind:   "plan.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "xyz"},
			{"name": "memory", "value": "9223372036854775807"},
			{"name": "swap", "value": "1024"},
			{"name": "cpushare", "value": "100"},
			{"name": "router", "value": "fake"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPlanAddWithMegabyteAsMemoryUnit(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=512M&swap=1024&cpushare=100&router=fake")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "xyz", Memory: 536870912, Swap: 1024, CpuShare: 100, Router: "fake"},
	})
}

func (s *S) TestPlanAddWithMegabyteAsSwapUnit(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=512M&swap=1024&cpushare=100&router=fake")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "xyz", Memory: 536870912, Swap: 1024, CpuShare: 100, Router: "fake"},
	})
}

func (s *S) TestPlanAddWithGigabyteAsMemoryUnit(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=512M&cpushare=100&router=fake")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "xyz", Memory: 9223372036854775807, Swap: 536870912, CpuShare: 100, Router: "fake"},
	})
}

func (s *S) TestPlanAddWithNoPermission(c *check.C) {
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=1&swap=2&cpushare=3")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanAddDupp(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader("name=xyz&memory=9223372036854775807&swap=1024&cpushare=100")
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	defer s.conn.Plans().RemoveAll(nil)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "xyz", Memory: 9223372036854775807, Swap: 1024, CpuShare: 100},
	})
	body = strings.NewReader("name=xyz&memory=9223372036854775807&swap=2&cpushare=3")
	recorder = httptest.NewRecorder()
	request, err = http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m = RunServer(true)
	m.ServeHTTP(recorder, request)
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
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestPlanList(c *check.C) {
	recorder := httptest.NewRecorder()
	expected := []app.Plan{
		{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3},
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	}
	err := s.conn.Plans().Insert(expected[0])
	c.Assert(err, check.IsNil)
	err = s.conn.Plans().Insert(expected[1])
	c.Assert(err, check.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	request, err := http.NewRequest("GET", "/plans", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var plans []app.Plan
	err = json.Unmarshal(recorder.Body.Bytes(), &plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, expected)
}

func (s *S) TestPlanRemove(c *check.C) {
	recorder := httptest.NewRecorder()
	expected := []app.Plan{
		{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3},
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	}
	err := s.conn.Plans().Insert(expected[0])
	c.Assert(err, check.IsNil)
	err = s.conn.Plans().Insert(expected[1])
	c.Assert(err, check.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	request, err := http.NewRequest("DELETE", "/plans/plan1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	})
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
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	plan := app.Plan{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	request, err := http.NewRequest("DELETE", "/plans/"+plan.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanRemoveInvalid(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/plans/plan999", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRoutersListNoContent(c *check.C) {
	err := config.Unset("routers")
	c.Assert(err, check.IsNil)
	defer config.Set("routers:fake:type", "fake")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestRoutersList(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	recorder := httptest.NewRecorder()
	expected := []router.PlanRouter{
		{Name: "fake", Type: "fake"},
		{Name: "fake-tls", Type: "fake-tls"},
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	request, err := http.NewRequest("GET", "/plans/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var routers []router.PlanRouter
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRoutersListNoPlanCreatePermission(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}
