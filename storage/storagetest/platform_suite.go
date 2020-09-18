// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type PlatformSuite struct {
	SuiteHooks
	PlatformStorage app.PlatformStorage
}

func (s *PlatformSuite) TestInsertPlatform(c *check.C) {
	p := app.Platform{Name: "python"}
	err := s.PlatformStorage.Insert(context.TODO(), p)
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformStorage.FindByName(context.TODO(), p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformSuite) TestInsertDuplicatePlatform(c *check.C) {
	t := app.Platform{Name: "java"}
	err := s.PlatformStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	err = s.PlatformStorage.Insert(context.TODO(), t)
	c.Assert(err, check.Equals, app.ErrDuplicatePlatform)
}

func (s *PlatformSuite) TestFindPlatformByName(c *check.C) {
	p := app.Platform{Name: "myteam"}
	err := s.PlatformStorage.Insert(context.TODO(), p)
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformStorage.FindByName(context.TODO(), p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformSuite) TestFindPlatformByNameNotFound(c *check.C) {
	platform, err := s.PlatformStorage.FindByName(context.TODO(), "wat")
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
	c.Assert(platform, check.IsNil)
}

func (s *PlatformSuite) TestFindAllPlatforms(c *check.C) {
	p1 := app.Platform{Name: "platform1"}
	err := s.PlatformStorage.Insert(context.TODO(), p1)
	c.Assert(err, check.IsNil)
	p2 := app.Platform{Name: "platform2"}
	err = s.PlatformStorage.Insert(context.TODO(), p2)
	c.Assert(err, check.IsNil)
	p3 := app.Platform{Name: "platform3"}
	err = s.PlatformStorage.Insert(context.TODO(), p3)
	c.Assert(err, check.IsNil)
	platforms, err := s.PlatformStorage.FindAll(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(platforms, check.DeepEquals, []app.Platform{p1, p2, p3})
}

func (s *PlatformSuite) TestFindEnabledPlatforms(c *check.C) {
	p1 := app.Platform{Name: "platform1"}
	err := s.PlatformStorage.Insert(context.TODO(), p1)
	c.Assert(err, check.IsNil)
	p2 := app.Platform{Name: "platform2", Disabled: true}
	err = s.PlatformStorage.Insert(context.TODO(), p2)
	c.Assert(err, check.IsNil)
	p3 := app.Platform{Name: "platform3", Disabled: false}
	err = s.PlatformStorage.Insert(context.TODO(), p3)
	c.Assert(err, check.IsNil)
	platforms, err := s.PlatformStorage.FindEnabled(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(platforms, check.DeepEquals, []app.Platform{p1, p3})
}

func (s *PlatformSuite) TestUpdatePlatform(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformStorage.Insert(context.TODO(), platform)
	c.Assert(err, check.IsNil)
	platform.Disabled = true
	err = s.PlatformStorage.Update(context.TODO(), platform)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformStorage.FindByName(context.TODO(), "static")
	c.Assert(err, check.IsNil)
	c.Assert(p.Disabled, check.Equals, true)
}

func (s *PlatformSuite) TestUpdatePlatformNotFound(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformStorage.Update(context.TODO(), platform)
	c.Assert(err, check.NotNil)
}

func (s *PlatformSuite) TestDeletePlatform(c *check.C) {
	platform := app.Platform{Name: "static"}
	err := s.PlatformStorage.Insert(context.TODO(), platform)
	c.Assert(err, check.IsNil)
	err = s.PlatformStorage.Delete(context.TODO(), platform)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformStorage.FindByName(context.TODO(), "static")
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
	c.Assert(p, check.IsNil)
}

func (s *PlatformSuite) TestDeletePlatformNotFound(c *check.C) {
	err := s.PlatformStorage.Delete(context.TODO(), app.Platform{Name: "myplatform"})
	c.Assert(err, check.Equals, app.ErrPlatformNotFound)
}
