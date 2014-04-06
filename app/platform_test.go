// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"launchpad.net/gocheck"
)

type PlatformSuite struct{}

var _ = gocheck.Suite(&PlatformSuite{})

func (s *PlatformSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "platform_tests")
}

func (s *PlatformSuite) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	conn.Apps().Database.DropDatabase()
	conn.Close()
}

func (s *PlatformSuite) TestPlatforms(c *gocheck.C) {
	want := []Platform{
		{Name: "dea"},
		{Name: "pecuniae"},
		{Name: "money"},
		{Name: "raise"},
		{Name: "glass"},
	}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	for _, p := range want {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, want)
}

func (s *PlatformSuite) TestPlatformsEmpty(c *gocheck.C) {
	got, err := Platforms()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.HasLen, 0)
}

func (s *PlatformSuite) TestGetPlatform(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	p := Platform{Name: "dea"}
	conn.Platforms().Insert(p)
	defer conn.Platforms().Remove(p)
	got, err := getPlatform(p.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, p)
	got, err = getPlatform("WAT")
	c.Assert(got, gocheck.IsNil)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, gocheck.Equals, true)
}
