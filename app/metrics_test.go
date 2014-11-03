// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/app/bind"
	"launchpad.net/gocheck"
)

func (s *S) TestMetricsEnabled(c *gocheck.C) {
	newApp := App{Name: "myApp", Platform: "Django"}
	c.Assert(hasMetricsEnabled(&newApp), gocheck.Equals, false)
	newApp = App{
		Name:     "myApp",
		Platform: "Django",
		Env: map[string]bind.EnvVar{
			"GRAPHITE_HOST": {
				Name:   "GRAPHITE_HOST",
				Value:  "host",
				Public: true,
			},
		},
	}
	c.Assert(hasMetricsEnabled(&newApp), gocheck.Equals, true)
}
