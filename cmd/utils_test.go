// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/tsuru/gnuflag"
	check "gopkg.in/check.v1"
)

func (s *S) TestMergeFlagSet(c *check.C) {
	var x, y bool
	fs1 := gnuflag.NewFlagSet("x", gnuflag.ExitOnError)
	fs1.BoolVar(&x, "x", false, "Something")
	fs2 := gnuflag.NewFlagSet("y", gnuflag.ExitOnError)
	fs2.BoolVar(&y, "y", false, "Something")
	ret := MergeFlagSet(fs1, fs2)
	c.Assert(ret, check.Equals, fs1)
	fs1.Parse(true, []string{"-x", "-y"})
	c.Assert(x, check.Equals, true)
	c.Assert(y, check.Equals, true)
}
