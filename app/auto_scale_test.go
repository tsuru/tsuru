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
	}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	s.provisioner.Provision(&newApp)
	defer s.provisioner.Destroy(&newApp)
	s.provisioner.AddUnits(&newApp, 2, nil)
	err = scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
}
