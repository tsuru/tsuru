// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"gopkg.in/check.v1"
)

func (s *S) TestMapFlag(c *check.C) {
	var f MapFlag
	f.Set("a=1")
	f.Set("b=2")
	f.Set("c=3")
	c.Assert(f, check.DeepEquals, MapFlag{
		"a": "1",
		"b": "2",
		"c": "3",
	})
}

func (s *S) TestStringSliceFlag(c *check.C) {
	var f StringSliceFlag
	f.Set("a")
	f.Set("b")
	f.Set("c")
	c.Assert(f, check.DeepEquals, StringSliceFlag{
		"a", "b", "c",
	})
}
