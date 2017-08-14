// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestInsertPlatform(c *check.C) {
	p := app.Platform{Name: "python"}
	err := s.PlatformService.Insert(p)
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformService.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *S) TestInsertDuplicatePlatform(c *check.C) {
	t := app.Platform{Name: "java"}
	err := s.PlatformService.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.PlatformService.Insert(t)
	c.Assert(err, check.Equals, app.ErrDuplicatePlatform)
}

func (s *S) TestFindPlatformByName(c *check.C) {
	p := app.Platform{Name: "myteam"}
	err := s.PlatformService.Insert(p)
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformService.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *S) TestFindPlatformByNameNotFound(c *check.C) {
	platform, err := s.PlatformService.FindByName("wat")
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
	c.Assert(platform, check.IsNil)
}

func (s *S) TestFindAllPlatforms(c *check.C) {
	p1 := app.Platform{Name: "platform1"}
	err := s.PlatformService.Insert(p1)
	c.Assert(err, check.IsNil)
	p2 := app.Platform{Name: "platform2"}
	err = s.PlatformService.Insert(p2)
	c.Assert(err, check.IsNil)
	p3 := app.Platform{Name: "platform3"}
	err = s.PlatformService.Insert(p3)
	c.Assert(err, check.IsNil)
	platforms, err := s.PlatformService.FindAll()
	c.Assert(err, check.IsNil)
	c.Assert(platforms, check.DeepEquals, []app.Platform{p1, p2, p3})
}

func (s *S) TestFindEnabledPlatforms(c *check.C) {
	p1 := app.Platform{Name: "platform1"}
	err := s.PlatformService.Insert(p1)
	c.Assert(err, check.IsNil)
	p2 := app.Platform{Name: "platform2", Disabled: true}
	err = s.PlatformService.Insert(p2)
	c.Assert(err, check.IsNil)
	p3 := app.Platform{Name: "platform3", Disabled: false}
	err = s.PlatformService.Insert(p3)
	c.Assert(err, check.IsNil)
	platforms, err := s.PlatformService.FindEnabled()
	c.Assert(err, check.IsNil)
	c.Assert(platforms, check.DeepEquals, []app.Platform{p1, p3})
}

func (s *S) TestUpdatePlatform(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformService.Insert(platform)
	c.Assert(err, check.IsNil)
	platform.Disabled = true
	err = s.PlatformService.Update(platform)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformService.FindByName("static")
	c.Assert(err, check.IsNil)
	c.Assert(p.Disabled, check.Equals, true)
}

func (s *S) TestUpdatePlatformNotFound(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformService.Update(platform)
	c.Assert(err, check.NotNil)
}

func (s *S) TestDeletePlatform(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformService.Insert(platform)
	c.Assert(err, check.IsNil)
	err = s.PlatformService.Delete(platform)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformService.FindByName("static")
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
	c.Assert(p, check.IsNil)
}

func (s *S) TestDeletePlatformNotFound(c *check.C) {
	err := s.PlatformService.Delete(app.Platform{Name: "myplatform"})
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
}
