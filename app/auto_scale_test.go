// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http/httptest"
	"time"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/mgo.v2"
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
			Enabled:  true,
		},
	}
	err := scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 0)
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
			Enabled:  true,
			MaxUnits: uint(10),
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
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, newApp.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].Error, gocheck.Equals, "")
	c.Assert(events[0].Successful, gocheck.Equals, true)
	c.Assert(events[0].AutoScaleConfig, gocheck.DeepEquals, newApp.AutoScaleConfig)
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
			Enabled:  true,
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
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "decrease")
	c.Assert(events[0].AppName, gocheck.Equals, newApp.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].Error, gocheck.Equals, "")
	c.Assert(events[0].Successful, gocheck.Equals, true)
	c.Assert(events[0].AutoScaleConfig, gocheck.DeepEquals, newApp.AutoScaleConfig)
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
			Enabled:  true,
			MaxUnits: uint(10),
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
			Enabled:  true,
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
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 2)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, up.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].Error, gocheck.Equals, "")
	c.Assert(events[0].Successful, gocheck.Equals, true)
	c.Assert(events[0].AutoScaleConfig, gocheck.DeepEquals, up.AutoScaleConfig)
	c.Assert(events[1].Type, gocheck.Equals, "decrease")
	c.Assert(events[1].AppName, gocheck.Equals, down.Name)
	c.Assert(events[1].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[1].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[1].Error, gocheck.Equals, "")
	c.Assert(events[1].Successful, gocheck.Equals, true)
	c.Assert(events[1].AutoScaleConfig, gocheck.DeepEquals, down.AutoScaleConfig)
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

func (s *S) TestValidateExpression(c *gocheck.C) {
	cases := map[string]bool{
		"{cpu} > 10": true,
		"{cpu} = 10": true,
		"{cpu} < 10": true,
		"cpu < 10":   false,
		"{cpu} 10":   false,
		"{cpu} <":    false,
		"{cpu}":      false,
		"<":          false,
		"100":        false,
	}
	for expression, expected := range cases {
		c.Assert(expressionIsValid(expression), gocheck.Equals, expected)
	}
}

func (s *S) TestNewAction(c *gocheck.C) {
	expression := "{cpu} > 10"
	units := uint(2)
	wait := time.Second
	a, err := NewAction(expression, units, wait)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Expression, gocheck.Equals, expression)
	c.Assert(a.Units, gocheck.Equals, units)
	c.Assert(a.Wait, gocheck.Equals, wait)
	expression = "{cpu} >"
	units = uint(2)
	wait = time.Second
	a, err = NewAction(expression, units, wait)
	c.Assert(err, gocheck.NotNil)
	c.Assert(a, gocheck.IsNil)
}

func (s *S) TestAutoScalebleApps(c *gocheck.C) {
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		AutoScaleConfig: &AutoScaleConfig{
			Enabled: true,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	disabledApp := App{
		Name:     "disabled",
		Platform: "Django",
		AutoScaleConfig: &AutoScaleConfig{
			Enabled: false,
		},
	}
	err = s.conn.Apps().Insert(disabledApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": disabledApp.Name})
	apps, err := autoScalableApps()
	c.Assert(err, gocheck.Equals, nil)
	c.Assert(apps[0].Name, gocheck.DeepEquals, newApp.Name)
	c.Assert(apps, gocheck.HasLen, 1)
}

func (s *S) TestLastScaleEvent(c *gocheck.C) {
	a := App{Name: "myApp", Platform: "Django"}
	event1, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	err = event1.update(nil)
	c.Assert(err, gocheck.IsNil)
	event2, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	event, err := lastScaleEvent(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(event.ID, gocheck.DeepEquals, event2.ID)
}

func (s *S) TestLastScaleEventNotFound(c *gocheck.C) {
	a := App{Name: "sam", Platform: "python"}
	_, err := lastScaleEvent(a.Name)
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}

func (s *S) TestListAutoScaleHistory(c *gocheck.C) {
	a := App{Name: "myApp", Platform: "Django"}
	_, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory("")
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, a.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
}

func (s *S) TestListAutoScaleHistoryByAppName(c *gocheck.C) {
	a := App{Name: "myApp", Platform: "Django"}
	_, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	a = App{Name: "another", Platform: "Django"}
	_, err = NewAutoScaleEvent(&a, "increase")
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory("another")
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, a.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
}

func (s *S) TestAutoScaleEnable(c *gocheck.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = AutoScaleEnable(&a)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.AutoScaleConfig.Enabled, gocheck.Equals, true)
}

func (s *S) TestAutoScaleDisable(c *gocheck.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = AutoScaleDisable(&a)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.AutoScaleConfig.Enabled, gocheck.Equals, false)
}

func (s *S) TestAutoScaleConfig(c *gocheck.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	config := AutoScaleConfig{
		Enabled:  true,
		MinUnits: 2,
		MaxUnits: 10,
	}
	err = SetAutoScaleConfig(&a, &config)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.AutoScaleConfig, gocheck.DeepEquals, &config)
}

func (s *S) TestAutoScaleUpWaitEventStillRunning(c *gocheck.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	app := App{
		Name:     "rush",
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
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 30e9},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, gocheck.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].ID, gocheck.DeepEquals, event.ID)
	c.Assert(app.Units(), gocheck.HasLen, 0)
}

func (s *S) TestAutoScaleUpWaitTime(c *gocheck.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	app := App{
		Name:     "rush",
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
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 1 * time.Hour},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, gocheck.IsNil)
	err = event.update(nil)
	c.Assert(err, gocheck.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].ID, gocheck.DeepEquals, event.ID)
	c.Assert(app.Units(), gocheck.HasLen, 0)
}

func (s *S) TestAutoScaleMaxUnits(c *gocheck.C) {
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
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80"},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Units(), gocheck.HasLen, 4)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "increase")
	c.Assert(events[0].AppName, gocheck.Equals, newApp.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].Error, gocheck.Equals, "")
	c.Assert(events[0].Successful, gocheck.Equals, true)
	c.Assert(events[0].AutoScaleConfig, gocheck.DeepEquals, newApp.AutoScaleConfig)
}

func (s *S) TestAutoScaleDownWaitEventStillRunning(c *gocheck.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	app := App{
		Name:     "rush",
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
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 30e9},
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20", Wait: 30e9},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "decrease")
	c.Assert(err, gocheck.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].ID, gocheck.DeepEquals, event.ID)
	c.Assert(app.Units(), gocheck.HasLen, 0)
}

func (s *S) TestAutoScaleDownWaitTime(c *gocheck.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	app := App{
		Name:     "rush",
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
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 1 * time.Hour},
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20", Wait: 3 * time.Hour},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, gocheck.IsNil)
	err = event.update(nil)
	c.Assert(err, gocheck.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, gocheck.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].ID, gocheck.DeepEquals, event.ID)
	c.Assert(app.Units(), gocheck.HasLen, 0)
}

func (s *S) TestAutoScaleMinUnits(c *gocheck.C) {
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
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20"},
			Enabled:  true,
			MinUnits: uint(3),
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 5, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Units(), gocheck.HasLen, 3)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, gocheck.IsNil)
	c.Assert(events, gocheck.HasLen, 1)
	c.Assert(events[0].Type, gocheck.Equals, "decrease")
	c.Assert(events[0].AppName, gocheck.Equals, newApp.Name)
	c.Assert(events[0].StartTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, gocheck.Not(gocheck.DeepEquals), time.Time{})
	c.Assert(events[0].Error, gocheck.Equals, "")
	c.Assert(events[0].Successful, gocheck.Equals, true)
	c.Assert(events[0].AutoScaleConfig, gocheck.DeepEquals, newApp.AutoScaleConfig)
}
