// Copyright 2015 tsuru authors. All rights reserved.
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
	_ "github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *S) TestPlanAdd(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"name": "xyz", "memory": 9223372036854775807, "swap": 1024, "cpushare": 100, "router": "fake" }`)
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
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
}

func (s *S) TestPlanAddWithNonAdmin(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"name": "xyz", "memory": 1, "swap": 2, "cpushare": 3 }`)
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanAddInvalidJson(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"name": "", "memory": 9223372036854775807, "swap": 1024, "cpushare": 100}`)
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)

	recorder = httptest.NewRecorder()
	body = strings.NewReader(`{"name": "xxx", "memory": 1234, "swap": 9999, "cpushare": 0}`)
	request, err = http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m = RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)

	recorder = httptest.NewRecorder()
	body = strings.NewReader(`{"name": "xxx", ".........`)
	request, err = http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m = RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestPlanAddDupp(c *check.C) {
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"name": "xyz", "memory": 9223372036854775807, "swap": 1024, "cpushare": 100 }`)
	request, err := http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
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
	body = strings.NewReader(`{"name": "xyz", "memory": 1, "swap": 2, "cpushare": 3 }`)
	recorder = httptest.NewRecorder()
	request, err = http.NewRequest("POST", "/plans", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m = RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
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
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var plans []app.Plan
	err = s.conn.Plans().Find(nil).All(&plans)
	c.Assert(err, check.IsNil)
	c.Assert(plans, check.DeepEquals, []app.Plan{
		{Name: "plan2", Memory: 3, Swap: 4, CpuShare: 5},
	})
}

func (s *S) TestPlanRemoveNonAdmin(c *check.C) {
	recorder := httptest.NewRecorder()
	plan := app.Plan{Name: "plan1", Memory: 1, Swap: 2, CpuShare: 3}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	defer s.conn.Plans().RemoveAll(nil)
	request, err := http.NewRequest("DELETE", "/plans/"+plan.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestPlanRemoveInvalid(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/plans/plan999", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRoutersList(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	recorder := httptest.NewRecorder()
	expected := []app.Router{
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	request, err := http.NewRequest("GET", "/plans/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.admintoken.GetValue())
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var routers []app.Router
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}
