// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import check "gopkg.in/check.v1"

func (s *S) TestURLPort(c *check.C) {
	var tests = []struct {
		input    string
		expected int
	}{
		{"http://192.168.50.4:3232", 3232},
		{"https://192.168.50.4:3232", 3232},
		{"https://192.168.50.4:5050", 5050},
		{"ftp://192.168.50.4:5050", 5050},
	}
	for _, t := range tests {
		c.Check(URLPort(t.input), check.Equals, t.expected)
	}
}
