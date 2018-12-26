// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"context"
	"time"

	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

func (s *S) TestInitializeDefault(c *check.C) {
	healer, err := Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(healer, check.NotNil)
	defer healer.Shutdown(context.Background())
	c.Assert(healer.waitTimeNewMachine, check.Equals, 5*60*time.Second)
	c.Assert(healer.disabledTime, check.Equals, 30*time.Second)
	c.Assert(healer.failuresBeforeHealing, check.Equals, 5)
}

func (s *S) TestInitializeDisabled(c *check.C) {
	config.Set("docker:healing:heal-nodes", false)
	defer config.Unset("docker:healing:heal-nodes")
	healer, err := Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(healer, check.IsNil)
}

func (s *S) TestInitializeConf(c *check.C) {
	config.Set("docker:healing:heal-nodes", true)
	config.Set("docker:healing:disabled-time", 10)
	config.Set("docker:healing:max-failures", 3)
	config.Set("docker:healing:wait-new-time", 2*60)
	defer config.Unset("docker:healing:heal-nodes")
	defer config.Unset("docker:healing:disabled-time")
	defer config.Unset("docker:healing:max-failures")
	defer config.Unset("docker:healing:wait-new-time")
	healer, err := Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(healer, check.NotNil)
	defer healer.Shutdown(context.Background())
	c.Assert(healer.waitTimeNewMachine, check.Equals, 2*60*time.Second)
	c.Assert(healer.disabledTime, check.Equals, 10*time.Second)
	c.Assert(healer.failuresBeforeHealing, check.Equals, 3)
}
