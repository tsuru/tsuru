// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestPluginInstallInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "plugin-install",
		Usage:   "plugin-install",
		Desc:    "Install tsuru plugins.",
		MinArgs: 0,
	}
	c.Assert(pluginInstal{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestPluginInstall(c *gocheck.C) {
	context := cmd.Context{}
	client := cmd.NewClient(nil, nil, manager)
	command := pluginInstal{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}
