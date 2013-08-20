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
	Register("my-provisioner", "my-healer", h)
	got, err := Get("my-provisioner", "my-healer")
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, h)
	_, err = Get("my-provisioner", "unknown-healer")
	c.Assert(err, gocheck.ErrorMatches, `Unknown healer "unknown-healer" for provisioner "my-provisioner".`)
}

func (s *S) TestGetWithAbsentProvisioner(c *gocheck.C) {
	var h Healer
	Register("provisioner", "healer1", h)
	h, err := Get("otherprovisioner", "healer1")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Unknown healer "healer1" for provisioner "otherprovisioner".`)
	c.Assert(h, gocheck.IsNil)
}

func (s *S) TestAllReturnsAllByCurrentProvisioner(c *gocheck.C) {
	var h Healer
	Register("provisioner", "healer1", h)
	Register("provisioner", "healer2", h)
	healers := All("provisioner")
	expected := map[string]Healer{
		"healer1": h,
		"healer2": h,
	}
	c.Assert(healers, gocheck.DeepEquals, expected)
}
