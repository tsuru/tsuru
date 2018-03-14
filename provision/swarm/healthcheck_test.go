// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestToHealthConfig(c *check.C) {
	tests := []struct {
		input    provision.TsuruYamlHealthcheck
		expected *container.HealthConfig
	}{
		{input: provision.TsuruYamlHealthcheck{}, expected: nil},
		{input: provision.TsuruYamlHealthcheck{
			Method: "PUT",
		}, expected: nil},
		{input: provision.TsuruYamlHealthcheck{
			Path:   "/",
			Method: "PUT",
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XPUT -fsSL http://localhost:9000/ -o/dev/null -w '%{http_code}' | grep 200",
			},
			Timeout:  120 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
		{input: provision.TsuruYamlHealthcheck{
			Path:   "/hc",
			Status: 201,
			Scheme: "https",
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL https://localhost:9000/hc -o/dev/null -w '%{http_code}' | grep 201",
			},
			Timeout:  120 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
		{input: provision.TsuruYamlHealthcheck{
			Path:            "hc",
			Status:          201,
			AllowedFailures: 10,
			Match:           "WORK.NG[0-9]+",
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL http://localhost:9000/hc | egrep \"WORK.NG[0-9]+\"",
			},
			Timeout:  120 * time.Second,
			Interval: 3 * time.Second,
			Retries:  11,
		}},
	}
	for i, test := range tests {
		result := toHealthConfig(test.input, 9000)
		c.Assert(result, check.DeepEquals, test.expected, check.Commentf("failed test %d", i))
	}
}
