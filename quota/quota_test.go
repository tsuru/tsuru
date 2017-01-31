// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(Suite{})

type Suite struct{}

func (Suite) TestQuotaExceededError(c *check.C) {
	err := QuotaExceededError{Requested: 10, Available: 9}
	c.Assert(err.Error(), check.Equals, "Quota exceeded. Available: 9. Requested: 10.")
}

func (Suite) TestQuotaUnlimited(c *check.C) {
	var q Quota
	q.Limit = -1
	c.Assert(q.Unlimited(), check.Equals, true)
	q.Limit = 0
	c.Assert(q.Unlimited(), check.Equals, false)
	q.Limit = 4
	c.Assert(q.Unlimited(), check.Equals, false)
}
