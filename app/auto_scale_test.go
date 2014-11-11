// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http/httptest"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestAutoScale(c *gocheck.C) {
	h := metricHandler{cpuMax: "50.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  ts.URL,
				Public: true,
			},
		},
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu} < 20"},
		},
	}
	err := scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestAutoScaleUp(c *gocheck.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  ts.URL,
				Public: true,
			},
		},
		Quota: quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Units(), gocheck.HasLen, 1)
}

func (s *S) TestAutoScaleDown(c *gocheck.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  ts.URL,
				Public: true,
			},
		},
		Quota: quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu_max} < 20"},
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 2, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Units(), gocheck.HasLen, 1)
}

func (s *S) TestRunAutoScaleOnce(c *gocheck.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	up := App{
		Name:     "myApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  ts.URL,
				Public: true,
			},
		},
		Quota: quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
		},
	}
	err := s.conn.Apps().Insert(up)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": up.Name})
	s.provisioner.Provision(&up)
	defer s.provisioner.Destroy(&up)
	dh := metricHandler{cpuMax: "9.2"}
	dts := httptest.NewServer(&dh)
	defer dts.Close()
	down := App{
		Name:     "anotherApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  dts.URL,
				Public: true,
			},
		},
		Quota: quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu_max} < 20"},
		},
	}
	err = s.conn.Apps().Insert(down)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": down.Name})
	s.provisioner.Provision(&down)
	defer s.provisioner.Destroy(&down)
	s.provisioner.AddUnits(&down, 3, nil)
	runAutoScaleOnce()
	c.Assert(up.Units(), gocheck.HasLen, 1)
	c.Assert(down.Units(), gocheck.HasLen, 2)
}

func (s *S) TestActionMetric(c *gocheck.C) {
	a := &Action{Expression: "{cpu} > 80"}
	c.Assert(a.metric(), gocheck.Equals, "cpu")
}

func (s *S) TestActionOperator(c *gocheck.C) {
	a := &Action{Expression: "{cpu} > 80"}
	c.Assert(a.operator(), gocheck.Equals, ">")
}

func (s *S) TestActionValue(c *gocheck.C) {
	a := &Action{Expression: "{cpu} > 80"}
	value, err := a.value()
	c.Assert(err, gocheck.IsNil)
	c.Assert(value, gocheck.Equals, float64(80))
}
