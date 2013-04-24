// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestApiCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "api",
		Usage:   "api",
		Desc:    "Starts the tsuru api webserver.",
		MinArgs: 0,
	}
	c.Assert(apiCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestApiCmdIsACommand(c *gocheck.C) {
	var _ cmd.Command = &apiCmd{}
}
