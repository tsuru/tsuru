// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gnuflag"
)

type configFile struct {
	value string
}

func (v *configFile) String() string {
	return v.value
}

func (v *configFile) Set(value string) error {
	err := config.ReadConfigFile(value)
	if err != nil {
		return err
	}
	v.value = value
	return nil
}

type tsrCommand struct {
	cmd.Command
	fs   *gnuflag.FlagSet
	file configFile
}

func (c *tsrCommand) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		if f, ok := c.Command.(cmd.FlaggedCommand); ok {
			c.fs = f.Flags()
		} else {
			c.fs = gnuflag.NewFlagSet("tsr", gnuflag.ExitOnError)
		}
		c.fs.Var(&c.file, "config", "Path to configuration file")
		c.fs.Var(&c.file, "c", "Path to configuration file")
	}
	return c.fs
}
