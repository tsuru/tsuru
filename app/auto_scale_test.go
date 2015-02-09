// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/quota"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAutoScale(c *check.C) {
	h := metricHandler{cpuMax: "50.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu} < 20"},
			Enabled:  true,
		},
	}
	err := scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestAutoScaleUp(c *check.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Enabled:  true,
			MaxUnits: uint(10),
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 1)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, newApp.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].AutoScaleConfig, check.DeepEquals, newApp.AutoScaleConfig)
}

func (s *S) TestAutoScaleDown(c *check.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu_max} < 20"},
			Enabled:  true,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 2, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 1)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "decrease")
	c.Assert(events[0].AppName, check.Equals, newApp.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].AutoScaleConfig, check.DeepEquals, newApp.AutoScaleConfig)
}

type autoscaleHandler struct {
	matches map[string]string
}

func (h *autoscaleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var cpu string
	for key, value := range h.matches {
		if strings.Contains(r.URL.String(), key) {
			cpu = value
		}
	}
	content := fmt.Sprintf(`[{"target": "sometarget", "datapoints": [[2.2, 1415129040], [2.2, 1415129050], [2.2, 1415129060], [2.2, 1415129070], [%s, 1415129080]]}]`, cpu)
	w.Write([]byte(content))
}

func (s *S) TestRunAutoScaleOnce(c *check.C) {
	h := autoscaleHandler{
		matches: map[string]string{
			"myApp":      "90.2",
			"anotherApp": "9.2",
		},
	}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	up := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Enabled:  true,
			MaxUnits: uint(10),
		},
	}
	err := s.conn.Apps().Insert(up)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": up.Name})
	s.provisioner.Provision(&up)
	defer s.provisioner.Destroy(&up)
	dh := metricHandler{cpuMax: "9.2"}
	dts := httptest.NewServer(&dh)
	defer dts.Close()
	down := App{
		Name:     "anotherApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu_max} < 20"},
			Enabled:  true,
		},
	}
	err = s.conn.Apps().Insert(down)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": down.Name})
	s.provisioner.Provision(&down)
	defer s.provisioner.Destroy(&down)
	s.provisioner.AddUnits(&down, 3, nil)
	runAutoScaleOnce()
	c.Assert(up.Units(), check.HasLen, 1)
	c.Assert(down.Units(), check.HasLen, 2)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 2)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, up.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].AutoScaleConfig, check.DeepEquals, up.AutoScaleConfig)
	c.Assert(events[1].Type, check.Equals, "decrease")
	c.Assert(events[1].AppName, check.Equals, down.Name)
	c.Assert(events[1].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[1].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[1].Error, check.Equals, "")
	c.Assert(events[1].Successful, check.Equals, true)
	c.Assert(events[1].AutoScaleConfig, check.DeepEquals, down.AutoScaleConfig)
}

func (s *S) TestActionMetric(c *check.C) {
	a := &Action{Expression: "{cpu} > 80"}
	c.Assert(a.metric(), check.Equals, "cpu")
}

func (s *S) TestActionOperator(c *check.C) {
	a := &Action{Expression: "{cpu} > 80"}
	c.Assert(a.operator(), check.Equals, ">")
}

func (s *S) TestActionValue(c *check.C) {
	a := &Action{Expression: "{cpu} > 80"}
	value, err := a.value()
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, float64(80))
}

func (s *S) TestValidateExpression(c *check.C) {
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
		c.Assert(expressionIsValid(expression), check.Equals, expected)
	}
}

func (s *S) TestNewAction(c *check.C) {
	expression := "{cpu} > 10"
	units := uint(2)
	wait := time.Second
	a, err := NewAction(expression, units, wait)
	c.Assert(err, check.IsNil)
	c.Assert(a.Expression, check.Equals, expression)
	c.Assert(a.Units, check.Equals, units)
	c.Assert(a.Wait, check.Equals, wait)
	expression = "{cpu} >"
	units = uint(2)
	wait = time.Second
	a, err = NewAction(expression, units, wait)
	c.Assert(err, check.NotNil)
	c.Assert(a, check.IsNil)
}

func (s *S) TestAutoScalebleApps(c *check.C) {
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		AutoScaleConfig: &AutoScaleConfig{
			Enabled: true,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	disabledApp := App{
		Name:     "disabled",
		Platform: "Django",
		AutoScaleConfig: &AutoScaleConfig{
			Enabled: false,
		},
	}
	err = s.conn.Apps().Insert(disabledApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": disabledApp.Name})
	apps, err := autoScalableApps()
	c.Assert(err, check.Equals, nil)
	c.Assert(apps[0].Name, check.DeepEquals, newApp.Name)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestLastScaleEvent(c *check.C) {
	a := App{Name: "myApp", Platform: "Django"}
	event1, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	event1.StartTime = event1.StartTime.Add(-1 * time.Hour)
	err = event1.update(nil)
	c.Assert(err, check.IsNil)
	event2, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	event, err := lastScaleEvent(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(event.ID, check.DeepEquals, event2.ID)
}

func (s *S) TestLastScaleEventNotFound(c *check.C) {
	a := App{Name: "sam", Platform: "python"}
	_, err := lastScaleEvent(a.Name)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestListAutoScaleHistory(c *check.C) {
	a := App{Name: "myApp", Platform: "Django"}
	_, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, a.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
}

func (s *S) TestListAutoScaleHistoryByAppName(c *check.C) {
	a := App{Name: "myApp", Platform: "Django"}
	_, err := NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	a = App{Name: "another", Platform: "Django"}
	_, err = NewAutoScaleEvent(&a, "increase")
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory("another")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, a.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
}

func (s *S) TestAutoScaleEnable(c *check.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = AutoScaleEnable(&a)
	c.Assert(err, check.IsNil)
	c.Assert(a.AutoScaleConfig.Enabled, check.Equals, true)
}

func (s *S) TestAutoScaleDisable(c *check.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = AutoScaleDisable(&a)
	c.Assert(err, check.IsNil)
	c.Assert(a.AutoScaleConfig.Enabled, check.Equals, false)
}

func (s *S) TestAutoScaleConfig(c *check.C) {
	a := App{Name: "myApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	config := AutoScaleConfig{
		Enabled:  true,
		MinUnits: 2,
		MaxUnits: 10,
	}
	err = SetAutoScaleConfig(&a, &config)
	c.Assert(err, check.IsNil)
	c.Assert(a.AutoScaleConfig, check.DeepEquals, &config)
}

func (s *S) TestAutoScaleUpWaitEventStillRunning(c *check.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	app := App{
		Name:     "rush",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 30e9},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, check.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].ID, check.DeepEquals, event.ID)
	c.Assert(app.Units(), check.HasLen, 0)
}

func (s *S) TestAutoScaleUpWaitTime(c *check.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	app := App{
		Name:     "rush",
		Platform: "Django",

		Quota: quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 1 * time.Hour},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, check.IsNil)
	err = event.update(nil)
	c.Assert(err, check.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].ID, check.DeepEquals, event.ID)
	c.Assert(app.Units(), check.HasLen, 0)
}

func (s *S) TestAutoScaleMaxUnits(c *check.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80"},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 4)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "increase")
	c.Assert(events[0].AppName, check.Equals, newApp.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].AutoScaleConfig, check.DeepEquals, newApp.AutoScaleConfig)
}

func (s *S) TestAutoScaleDownWaitEventStillRunning(c *check.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	app := App{
		Name:     "rush",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 30e9},
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20", Wait: 30e9},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "decrease")
	c.Assert(err, check.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].ID, check.DeepEquals, event.ID)
	c.Assert(app.Units(), check.HasLen, 0)
}

func (s *S) TestAutoScaleDownWaitTime(c *check.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	app := App{
		Name:     "rush",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 5, Expression: "{cpu_max} > 80", Wait: 1 * time.Hour},
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20", Wait: 3 * time.Hour},
			Enabled:  true,
			MaxUnits: 4,
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	event, err := NewAutoScaleEvent(&app, "increase")
	c.Assert(err, check.IsNil)
	err = event.update(nil)
	c.Assert(err, check.IsNil)
	err = scaleApplicationIfNeeded(&app)
	c.Assert(err, check.IsNil)
	events, err := ListAutoScaleHistory(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].ID, check.DeepEquals, event.ID)
	c.Assert(app.Units(), check.HasLen, 0)
}

func (s *S) TestAutoScaleMinUnits(c *check.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 3, Expression: "{cpu_max} < 20"},
			Enabled:  true,
			MinUnits: uint(3),
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 5, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 3)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Type, check.Equals, "decrease")
	c.Assert(events[0].AppName, check.Equals, newApp.Name)
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].AutoScaleConfig, check.DeepEquals, newApp.AutoScaleConfig)
}

func (s *S) TestAutoScaleConfigMarshalJSON(c *check.C) {
	conf := &AutoScaleConfig{
		Increase: Action{Units: 1, Expression: "{cpu} > 80"},
		Decrease: Action{Units: 1, Expression: "{cpu} < 20"},
		Enabled:  true,
		MaxUnits: 10,
		MinUnits: 2,
	}
	expected := map[string]interface{}{
		"increase": map[string]interface{}{
			"wait":       float64(0),
			"expression": "{cpu} > 80",
			"units":      float64(1),
		},
		"decrease": map[string]interface{}{
			"wait":       float64(0),
			"expression": "{cpu} < 20",
			"units":      float64(1),
		},
		"minUnits": float64(2),
		"maxUnits": float64(10),
		"enabled":  true,
	}
	data, err := json.Marshal(conf)
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAutoScaleDownMin(c *check.C) {
	h := metricHandler{cpuMax: "10.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu_max} < 20"},
			Enabled:  true,
			MinUnits: 1,
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 1, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 1)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestAutoScaleUpMax(c *check.C) {
	h := metricHandler{cpuMax: "90.2"}
	ts := httptest.NewServer(&h)
	config.Set("metrics:db", "graphite")
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	newApp := App{
		Name:     "myApp",
		Platform: "Django",
		Quota:    quota.Unlimited,
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu_max} > 80"},
			Enabled:  true,
			MaxUnits: uint(2),
		},
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 2, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Units(), check.HasLen, 2)
	var events []AutoScaleEvent
	err = s.conn.AutoScale().Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}
