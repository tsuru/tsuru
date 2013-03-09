// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heal

import (
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestRegisterAndGetHealer(c *gocheck.C) {
	var h Healer
	Register("my-healer", h)
	got, err := Get("my-healer")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, h)
	_, err = Get("unknown-healer")
	c.Assert(err, gocheck.ErrorMatches, `Unknown healer: "unknown-healer".`)
}

func (s *S) TestAll(c *gocheck.C) {
	var h Healer
	Register("healer1", h)
	Register("healer2", h)
	healers := All()
	expected := map[string]Healer{
		"healer1": h,
		"healer2": h,
	}
	c.Assert(healers, gocheck.DeepEquals, expected)
}
