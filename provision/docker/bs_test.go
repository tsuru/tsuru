// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetImageFromDatabase(c *check.C) {
	imageName := "tsuru/bsss"
	coll, err := bsCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(bsConfig{Image: imageName})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"image": imageName})
	image, err := getBsImage()
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageFromConfig(c *check.C) {
	imageName := "tsuru/bs:v10"
	config.Set("docker:bs:image", imageName)
	image, err := getBsImage()
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageDefaultValue(c *check.C) {
	config.Unset("docker:bs:image")
	image, err := getBsImage()
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "tsuru/bs")
}

func (s *S) TestSaveImage(c *check.C) {
	coll, err := bsCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = saveBsImage("tsuru/bs@sha1:afd533420cf")
	c.Assert(err, check.IsNil)
	var configs []bsConfig
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420cf")
	err = saveBsImage("tsuru/bs@sha1:afd533420d0")
	c.Assert(err, check.IsNil)
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420d0")
}
