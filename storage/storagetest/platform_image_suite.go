// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/types/app/image"
	"gopkg.in/check.v1"
)

type PlatformImageSuite struct {
	SuiteHooks
	PlatformImageStorage image.PlatformImageStorage
}

func (s *PlatformImageSuite) TestPlatformImageUpsert(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert("myplatform")
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformImageSuite) TestPlatformImageFindByName(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert("myplatform")
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName(p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformImageSuite) TestFindPlatformByNameNotFound(c *check.C) {
	platform, err := s.PlatformImageStorage.FindByName("wat")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
	c.Assert(platform, check.IsNil)
}

func (s *PlatformImageSuite) TestPlatformImageAppend(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert("myplatform")
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Append(p.Name, "tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(platform.Images, check.DeepEquals, []string{"tsuru/myplatform:v1"})
}

func (s *PlatformImageSuite) TestDeletePlatform(c *check.C) {
	platform, err := s.PlatformImageStorage.Upsert("static")
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Delete(platform.Name)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformImageStorage.FindByName("static")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
	c.Assert(p, check.IsNil)
}

func (s *PlatformImageSuite) TestDeletePlatformNotFound(c *check.C) {
	err := s.PlatformImageStorage.Delete("myplatform")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
}
