// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gnuflag"
)

type adminCmd struct {
	fs  *gnuflag.FlagSet
	dry bool
}

func (adminCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "admin-api",
		Usage:   "admin-api",
		Desc:    "Starts the tsuru admin api webserver.",
		MinArgs: 0,
	}
}

func (c *adminCmd) Run(context *cmd.Context, client *cmd.Client) error {
	api.RunAdminServer(c.dry)
	return nil
}

func (c *adminCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("api", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.dry, "dry", false, "dry-run: does not start the server (for testing purpose)")
		c.fs.BoolVar(&c.dry, "d", false, "dry-run: does not start the server (for testing purpose)")
	}
	return c.fs
}
