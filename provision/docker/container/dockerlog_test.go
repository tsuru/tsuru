// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"github.com/tsuru/tsuru/scopedconfig"
	"gopkg.in/check.v1"
)

func (s *S) TestDockerLogUpdate(c *check.C) {
	testCases := []struct {
		pool   string
		conf   DockerLogConfig
		result map[string]DockerLogConfig
		err    error
	}{
		{
			"",
			DockerLogConfig{Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:24224"}},
			map[string]DockerLogConfig{
				"": {Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:24224"}},
			},
			nil,
		},
		{
			"",
			DockerLogConfig{Driver: "bs", LogOpts: map[string]string{"tag": "ahoy"}},
			map[string]DockerLogConfig{
				"": {Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:24224"}},
			},
			ErrLogDriverBSNoParams,
		},
		{
			"",
			DockerLogConfig{Driver: "", LogOpts: map[string]string{"tag": "ahoy"}},
			map[string]DockerLogConfig{
				"": {Driver: "fluentd", LogOpts: map[string]string{"fluentd-address": "localhost:24224"}},
			},
			ErrLogDriverMandatory,
		},
		{
			"",
			DockerLogConfig{Driver: "bs", LogOpts: nil},
			map[string]DockerLogConfig{
				"": {Driver: "bs", LogOpts: map[string]string{}},
			},
			nil,
		},
		{
			"",
			DockerLogConfig{Driver: "fluentd", LogOpts: map[string]string{"tag": "x"}},
			map[string]DockerLogConfig{
				"": {Driver: "fluentd", LogOpts: map[string]string{"tag": "x"}},
			},
			nil,
		},
		{
			"p1",
			DockerLogConfig{Driver: "journald", LogOpts: map[string]string{"tag": "y"}},
			map[string]DockerLogConfig{
				"":   {Driver: "fluentd", LogOpts: map[string]string{"tag": "x"}},
				"p1": {Driver: "journald", LogOpts: map[string]string{"tag": "y"}},
			},
			nil,
		},
	}
	for _, testData := range testCases {
		err := testData.conf.Save(testData.pool)
		c.Assert(err, check.DeepEquals, testData.err)
		conf := scopedconfig.FindScopedConfig(dockerLogConfigCollection)
		var all map[string]DockerLogConfig
		err = conf.LoadAll(&all)
		c.Assert(err, check.IsNil)
		c.Assert(all, check.DeepEquals, testData.result)
	}
	driver, opts, err := LogOpts("p1")
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.Equals, "journald")
	c.Assert(opts, check.DeepEquals, map[string]string{"tag": "y"})
	driver, opts, err = LogOpts("other")
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.Equals, "fluentd")
	c.Assert(opts, check.DeepEquals, map[string]string{"tag": "x"})
}
