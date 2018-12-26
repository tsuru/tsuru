// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestURLtoHost(c *check.C) {
	tests := map[string]string{
		"http://myhost.com":    "myhost.com",
		"http://localhost":     "localhost",
		"http://localhost:123": "localhost",
		"localhost":            "localhost",
		"localhost:123":        "localhost",
	}
	for address, host := range tests {
		c.Check(URLToHost(address), check.Equals, host)
	}
}
