// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestTokenCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "token",
		Usage:   "token",
		Desc:    "Generates a tsuru token.",
		MinArgs: 0,
	}
	c.Assert(tokenCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTokenCmdIsACommand(c *gocheck.C) {
	var _ cmd.Command = &tokenCmd{}
}
