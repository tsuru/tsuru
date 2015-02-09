// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"testing"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type influx struct{}

func (i influx) Summarize(key, interval, function string) (Series, error) {
	return nil, nil
}

func (s *S) TestRegister(c *check.C) {
	Register("influx", influx{})
	db, ok := dbs["influx"]
	c.Assert(ok, check.Equals, true)
	c.Assert(db, check.FitsTypeOf, influx{})
}

func (s *S) TestGet(c *check.C) {
	_, err := Get()
	c.Assert(err, check.Not(check.IsNil))
	config.Set("metrics:db", "influx")
	_, err = Get()
	c.Assert(err, check.Not(check.IsNil))
	Register("influx", influx{})
	db, err := Get()
	c.Assert(err, check.IsNil)
	c.Assert(db, check.FitsTypeOf, influx{})
}
