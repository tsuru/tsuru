// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestPlatformNewImage(c *check.C) {
	img1, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/myplatform:v1")
	img2, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/myplatform:v2")
	img3, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "tsuru/myplatform:v3")
}

func (s *S) TestPlatformNewImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	img1, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/myplatform:v1")
	img2, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "localhost:3030/tsuru/myplatform:v2")
	img3, err := PlatformNewImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "localhost:3030/tsuru/myplatform:v3")
}

func (s *S) TestPlatformCurrentImage(c *check.C) {
	img, err := PlatformCurrentImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/myplatform:latest")
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	img1, err := PlatformCurrentImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/myplatform:v1")
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v2")
	c.Assert(err, check.IsNil)
	img2, err := PlatformCurrentImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/myplatform:v2")
}

func (s *S) TestPlatformCurrentImageWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	err := PlatformAppendImage("myplatform", "localhost:3030/tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	img1, err := PlatformCurrentImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/myplatform:v1")
	err = PlatformAppendImage("myplatform", "localhost:3030/tsuru/myplatform:v2")
	c.Assert(err, check.IsNil)
	img2, err := PlatformCurrentImage("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "localhost:3030/tsuru/myplatform:v2")
}

func (s *S) TestPlatformListImages(c *check.C) {
	err := PlatformAppendImage("myplatform", "tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v2")
	c.Assert(err, check.IsNil)
	images, err := PlatformListImages("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2"})
}

func (s *S) TestPlatformDeleteImages(c *check.C) {
	err := PlatformAppendImage("myplatform", "tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v2")
	c.Assert(err, check.IsNil)
	err = PlatformDeleteImages("myplatform")
	c.Assert(err, check.IsNil)
	_, err = PlatformListImages("myplatform")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestPlatformAppendImage(c *check.C) {
	err := PlatformAppendImage("myplatform", "tsuru/myplatform:v1")
	c.Assert(err, check.IsNil)
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v2")
	c.Assert(err, check.IsNil)
	err = PlatformAppendImage("myplatform", "tsuru/myplatform:v3")
	c.Assert(err, check.IsNil)
	images, err := PlatformListImages("myplatform")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/myplatform:v1", "tsuru/myplatform:v2", "tsuru/myplatform:v3"})
}
