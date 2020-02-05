// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	check "gopkg.in/check.v1"
)

func (s *S) TestGetProcessesFromProcfile(c *check.C) {
	tests := []struct {
		procfile string
		expected map[string][]string
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
	}
	for i, t := range tests {
		v := GetProcessesFromProcfile(t.procfile)
		c.Check(v, check.DeepEquals, t.expected, check.Commentf("failed test %d", i))
	}
}
