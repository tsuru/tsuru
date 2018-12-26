// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/tsuru/tsuru/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestToHealthConfig(c *check.C) {
	tests := []struct {
		input    provision.TsuruYamlData
		expected *container.HealthConfig
	}{
		{input: provision.TsuruYamlData{}, expected: nil},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Method: "PUT",
			},
		}, expected: nil},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Path:   "/",
				Method: "PUT",
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XPUT -fsSL http://localhost:9000/ -o/dev/null -w '%{http_code}' | grep 200",
			},
			Timeout:  60 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Path:   "/hc",
				Status: 201,
				Scheme: "https",
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL https://localhost:9000/hc -o/dev/null -w '%{http_code}' | grep 201",
			},
			Timeout:  60 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Path:            "hc",
				Status:          201,
				AllowedFailures: 10,
				Match:           "WORK.NG[0-9]+",
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL http://localhost:9000/hc | egrep \"WORK.NG[0-9]+\"",
			},
			Timeout:  60 * time.Second,
			Interval: 3 * time.Second,
			Retries:  11,
		}},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Path:           "hc",
				TimeoutSeconds: 99,
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL http://localhost:9000/hc -o/dev/null -w '%{http_code}' | grep 200",
			},
			Timeout:  99 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
		{input: provision.TsuruYamlData{
			Healthcheck: provision.TsuruYamlHealthcheck{
				Path:            "hc",
				Status:          201,
				AllowedFailures: 10,
				Match:           "WORK.NG[0-9]+",
			},
			Hooks: provision.TsuruYamlHooks{
				Restart: provision.TsuruYamlRestartHooks{
					After: []string{"my cmd 1", "my cmd 2"},
				},
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"curl -k -XGET -fsSL http://localhost:9000/hc | egrep \"WORK.NG[0-9]+\" && if [ ! -f /tmp/restartafterok ]; then my cmd 1 && my cmd 2 && touch /tmp/restartafterok; fi",
			},
			Timeout:  60 * time.Second,
			Interval: 3 * time.Second,
			Retries:  11,
		}},
		{input: provision.TsuruYamlData{
			Hooks: provision.TsuruYamlHooks{
				Restart: provision.TsuruYamlRestartHooks{
					After: []string{"my cmd 1", "my cmd 2"},
				},
			},
		}, expected: &container.HealthConfig{
			Test: []string{
				"CMD-SHELL",
				"if [ ! -f /tmp/restartafterok ]; then my cmd 1 && my cmd 2 && touch /tmp/restartafterok; fi",
			},
			Timeout:  60 * time.Second,
			Interval: 3 * time.Second,
			Retries:  1,
		}},
	}
	for i, test := range tests {
		result := toHealthConfig(test.input, 9000)
		c.Assert(result, check.DeepEquals, test.expected, check.Commentf("failed test %d", i))
	}
}
