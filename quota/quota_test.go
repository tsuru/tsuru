// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import (
	"testing"

	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

var _ = gocheck.Suite(Suite{})

type Suite struct{}

func (Suite) TestQuotaExceededError(c *gocheck.C) {
	err := QuotaExceededError{Requested: 10, Available: 9}
	c.Assert(err.Error(), gocheck.Equals, "Quota exceeded. Available: 9. Requested: 10.")
}
