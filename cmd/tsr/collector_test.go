// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestCollectorCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "collector",
		Usage:   "collector",
		Desc:    "Starts the tsuru collector.",
		MinArgs: 0,
	}
	c.Assert(collectorCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestCollectorCmdIsACommand(c *gocheck.C) {
	var _ cmd.Command = &collectorCmd{}
}
