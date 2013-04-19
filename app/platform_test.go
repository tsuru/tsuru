// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"launchpad.net/gocheck"
)

func (s *S) TestPlatforms(c *gocheck.C) {
	want := []Platform{
		{Name: "dea"},
		{Name: "pecuniae"},
		{Name: "money"},
		{Name: "raise"},
		{Name: "glass"},
	}
	for _, p := range want {
		s.conn.Platforms().Insert(p)
		defer s.conn.Platforms().Remove(p)
	}
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, want)
}

func (s *S) TestPlatformsEmpty(c *gocheck.C) {
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.HasLen, 0)
}

func (s *S) TestGetPlatform(c *gocheck.C) {
	p := Platform{Name: "dea"}
	s.conn.Platforms().Insert(p)
	defer s.conn.Platforms().Remove(p)
	got, err := getPlatform(p.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, p)
	got, err = getPlatform("WAT")
	c.Assert(got, gocheck.IsNil)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, gocheck.Equals, true)
}
