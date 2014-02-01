// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestPlan(c *gocheck.C) {
	plan := Plan{
		Name:        "Ignite",
		Description: "A simple plan",
	}
	c.Assert("Ignite", gocheck.Equals, plan.Name)
	c.Assert("A simple plan", gocheck.Equals, plan.Description)
}

func (s *S) TestGetPlansByServiceName(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srvc)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srvc.Name)
	plans, err := GetPlansByServiceName("mysql")
	c.Assert(err, gocheck.IsNil)
	expected := []Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(plans, gocheck.DeepEquals, expected)
}
