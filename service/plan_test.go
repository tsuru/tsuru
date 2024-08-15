// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/db/storagev2"
	check "gopkg.in/check.v1"
)

func (s *S) TestPlan(c *check.C) {
	plan := Plan{
		Name:        "Ignite",
		Description: "A simple plan",
	}
	c.Assert("Ignite", check.Equals, plan.Name)
	c.Assert("A simple plan", check.Equals, plan.Description)
}

func (s *S) TestGetPlansByServiceName(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}

	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)

	plans, err := GetPlansByService(context.TODO(), srvc, "", "")
	c.Assert(err, check.IsNil)
	expected := []Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(plans, check.DeepEquals, expected)
}

func (s *S) TestGetPlanByServiceNameAndPlanName(c *check.C) {
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
		w.Write([]byte(content))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)
	plan, err := GetPlanByServiceAndPlanName(context.TODO(), srvc, "", "small", "")
	c.Assert(err, check.IsNil)
	expected := Plan{
		Name:        "small",
		Description: "not space left for you",
	}
	c.Assert(plan, check.DeepEquals, expected)
}

func (s *S) TestGetPlansByServiceNameWithoutEndpoint(c *check.C) {
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)

	srvc := Service{Name: "mysql"}
	_, err = servicesCollection.InsertOne(context.TODO(), &srvc)
	c.Assert(err, check.IsNil)

	plans, err := GetPlansByService(context.TODO(), srvc, "", "")
	c.Assert(err, check.IsNil)
	expected := []Plan{}
	c.Assert(plans, check.DeepEquals, expected)
}
