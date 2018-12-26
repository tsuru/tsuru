// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

func (s *S) TestBsSysLogPort(c *check.C) {
	c.Check(BsSysLogPort(), check.Equals, 1514)
	config.Set("docker:bs:syslog-port", 1515)
	defer config.Unset("docker:bs:syslog-port")
	c.Check(BsSysLogPort(), check.Equals, 1515)
}
