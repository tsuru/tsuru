// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsurutest

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

var _ = check.Suite(S{})

type S struct{}

func (S) TestWaitConditionSuccess(c *check.C) {
	err := WaitCondition(1e9, func() bool {
		return true
	})
	c.Assert(err, check.IsNil)
}

func (S) TestWaitConditionTimeout(c *check.C) {
	err := WaitCondition(30e6, func() bool {
		return false
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "timed out waiting for condition after 30ms")
}
