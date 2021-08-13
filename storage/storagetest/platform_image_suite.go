// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/types/app/image"
	check "gopkg.in/check.v1"
)

type PlatformImageSuite struct {
	SuiteHooks
	PlatformImageStorage image.PlatformImageStorage
}

func (s *PlatformImageSuite) TestPlatformImageUpsert(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert(context.TODO(), "myplatform")
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName(context.TODO(), p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformImageSuite) TestPlatformImageFindByName(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert(context.TODO(), "myplatform")
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName(context.TODO(), p.Name)
	c.Assert(err, check.IsNil)
	c.Assert(platform.Name, check.Equals, p.Name)
}

func (s *PlatformImageSuite) TestFindPlatformByNameNotFound(c *check.C) {
	platform, err := s.PlatformImageStorage.FindByName(context.TODO(), "wat")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
	c.Assert(platform, check.IsNil)
}

func (s *PlatformImageSuite) TestPlatformImageAppend(c *check.C) {
	p, err := s.PlatformImageStorage.Upsert(context.TODO(), "myplatform")
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Append(context.TODO(), p.Name, 1, []string{"tsuru/myplatform:v1"})
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Append(context.TODO(), p.Name, 1, []string{"other.registry/tsuru/myplatform:v1"})
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Append(context.TODO(), p.Name, 2, []string{"tsuru/myplatform:v2"})
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Append(context.TODO(), p.Name, 3, []string{"tsuru/myplatform:v3", "registy.tsuru/plat:v3"})
	c.Assert(err, check.IsNil)
	platform, err := s.PlatformImageStorage.FindByName(context.TODO(), "myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(platform.Versions, check.DeepEquals, []image.RegistryVersion{
		{
			Version: 1,
			Images:  []string{"tsuru/myplatform:v1", "other.registry/tsuru/myplatform:v1"},
		},
		{
			Version: 2,
			Images:  []string{"tsuru/myplatform:v2"},
		},
		{
			Version: 3,
			Images:  []string{"tsuru/myplatform:v3", "registy.tsuru/plat:v3"},
		},
	})
}

func (s *PlatformImageSuite) TestDeletePlatform(c *check.C) {
	platform, err := s.PlatformImageStorage.Upsert(context.TODO(), "static")
	c.Assert(err, check.IsNil)
	err = s.PlatformImageStorage.Delete(context.TODO(), platform.Name)
	c.Assert(err, check.IsNil)
	p, err := s.PlatformImageStorage.FindByName(context.TODO(), "static")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
	c.Assert(p, check.IsNil)
}

func (s *PlatformImageSuite) TestDeletePlatformNotFound(c *check.C) {
	err := s.PlatformImageStorage.Delete(context.TODO(), "myplatform")
	c.Assert(err, check.Equals, image.ErrPlatformImageNotFound)
}
