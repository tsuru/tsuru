// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestSwapInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "swap",
		Usage:   "swap app1-name app2-name",
		Desc:    "Swap router between two apps.",
		MinArgs: 2,
	}
	c.Assert(swap{}.Info(), gocheck.DeepEquals, expected)
}
