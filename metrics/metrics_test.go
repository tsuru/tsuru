// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"testing"

	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

type influx struct{}

func (i influx) Summarize(key, interval, function string) (Series, error) {
	return nil, nil
}

func (s *S) TestRegister(c *gocheck.C) {
	Register("influx", influx{})
	db, ok := dbs["influx"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(db, gocheck.FitsTypeOf, influx{})
}
