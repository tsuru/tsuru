// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/heal"
	"launchpad.net/gnuflag"
	"time"
)

type healerCmd struct {
	fs   *gnuflag.FlagSet
	host string
}

func (c *healerCmd) Run(context *cmd.Context, client *cmd.Client) error {
	heal.RegisterHealerTicker(time.Tick(time.Minute*15), c.host)
	heal.HealTicker(time.Tick(time.Minute))
	return nil
}

func (c *healerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "healer",
		Usage:   "healer --host <tsuru-host:port>",
		Desc:    "Starts tsuru healer agent.",
		MinArgs: 1,
	}
}

func (c *healerCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("healer", gnuflag.ExitOnError)
		c.fs.StringVar(&c.host, "host", "", "host: tsuru host to discover and call healers on")
		c.fs.StringVar(&c.host, "h", "", "host: tsuru host to discover and call healers on")
	}
	return c.fs
}
