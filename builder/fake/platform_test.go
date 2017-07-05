// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"github.com/tsuru/tsuru/builder"
	"gopkg.in/check.v1"
)

func (s *S) TestPlatformAdd(c *check.C) {
	p := FakeBuilder{}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd(builder.PlatformOptions{Name: "python", Args: args})
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, check.Equals, "python")
	c.Assert(platform.Version, check.Equals, 1)
	c.Assert(platform.Args, check.DeepEquals, args)
}

func (s *S) TestPlatformAddTwice(c *check.C) {
	p := FakeBuilder{}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd(builder.PlatformOptions{Name: "python", Args: args})
	c.Assert(err, check.IsNil)
	err = p.PlatformAdd(builder.PlatformOptions{Name: "python", Args: nil})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "duplicate platform")
}

func (s *S) TestPlatformUpdate(c *check.C) {
	p := FakeBuilder{}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd(builder.PlatformOptions{Name: "python", Args: args})
	c.Assert(err, check.IsNil)
	args["something"] = "wat"
	err = p.PlatformUpdate(builder.PlatformOptions{Name: "python", Args: args})
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, check.Equals, "python")
	c.Assert(platform.Version, check.Equals, 2)
	c.Assert(platform.Args, check.DeepEquals, args)
}

func (s *S) TestPlatformUpdateNotFound(c *check.C) {
	p := FakeBuilder{}
	err := p.PlatformUpdate(builder.PlatformOptions{Name: "python", Args: nil})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "platform not found")
}

func (s *S) TestPlatformRemove(c *check.C) {
	p := FakeBuilder{}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd(builder.PlatformOptions{Name: "python", Args: args})
	c.Assert(err, check.IsNil)
	err = p.PlatformRemove("python")
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform, check.IsNil)
}

func (s *S) TestPlatformRemoveNotFound(c *check.C) {
	p := FakeBuilder{}
	err := p.PlatformRemove("python")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "platform not found")
}
