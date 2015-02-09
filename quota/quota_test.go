// Copyright 2015 tsuru authors. All rights reserved.
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
