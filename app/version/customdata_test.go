// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestGetProcessesFromProcfile(c *check.C) {
	tests := []struct {
		expected map[string][]string
		procfile string
	}{
		{procfile: "", expected: map[string][]string{}},
		{procfile: "invalid", expected: map[string][]string{}},
		{procfile: "web: a b c", expected: map[string][]string{
			"web": {"a b c"},
		}},
		{procfile: "web: a b c\nworker: \t  x y z \r  ", expected: map[string][]string{
			"web":    {"a b c"},
			"worker": {"x y z"},
		}},
		{procfile: "web:abc\nworker:xyz", expected: map[string][]string{
			"web":    {"abc"},
			"worker": {"xyz"},
		}},
		{procfile: "web: a b c\r\nworker:x\r\nworker2: z\r\n", expected: map[string][]string{
			"web":     {"a b c"},
			"worker":  {"x"},
			"worker2": {"z"},
		}},
		{procfile: "", expected: map[string][]string{}},
		{procfile: "web:", expected: map[string][]string{}},
	}
	for i, t := range tests {
		v := GetProcessesFromProcfile(t.procfile)
		c.Check(v, check.DeepEquals, t.expected, check.Commentf("failed test %d", i))
	}
}

func (s *S) TestGetProcessesFromYamlProcess(c *check.C) {
	tests := []struct {
		expected  map[string][]string
		processes []provTypes.TsuruYamlProcess
	}{
		{processes: nil, expected: map[string][]string{}},
		{processes: []provTypes.TsuruYamlProcess{}, expected: map[string][]string{}},
		{
			processes: []provTypes.TsuruYamlProcess{{Name: "web", Command: "python app.py"}},
			expected: map[string][]string{
				"web": {"python app.py"},
			},
		},
		{
			processes: []provTypes.TsuruYamlProcess{{Name: "web", Command: "python app.py"}, {Name: "worker", Command: "python worker.py"}},
			expected: map[string][]string{
				"web":    {"python app.py"},
				"worker": {"python worker.py"},
			},
		},
		{
			processes: []provTypes.TsuruYamlProcess{{Name: "web", Command: ""}, {Name: "worker", Command: "python worker.py"}},
			expected: map[string][]string{
				"worker": {"python worker.py"},
			},
		},
	}
	for i, t := range tests {
		v := GetProcessesFromYamlProcess(t.processes)
		c.Check(v, check.DeepEquals, t.expected, check.Commentf("failed test %d", i))
	}
}
