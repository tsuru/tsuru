// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/cmd"
)

type configFile struct {
	value string
}

func (v *configFile) String() string {
	return v.value
}

func (v *configFile) Set(value string) error {
	v.value = value
	configPath = value
	return nil
}

type tsurudCommand struct {
	cmd.Command
	fs   *gnuflag.FlagSet
	file configFile
}

func (c *tsurudCommand) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		if f, ok := c.Command.(cmd.FlaggedCommand); ok {
			c.fs = f.Flags()
		} else {
			c.fs = gnuflag.NewFlagSet("tsurud", gnuflag.ExitOnError)
		}
		c.fs.Var(&c.file, "config", "Path to configuration file (default to /etc/tsuru/tsuru.conf)")
		c.fs.Var(&c.file, "c", "Path to configuration file (default to /etc/tsuru/tsuru.conf)")
	}
	return c.fs
}

func (c *tsurudCommand) Run(context *cmd.Context) error {
	fmt.Fprintf(context.Stderr, "Opening config file: %s\n", configPath)
	err := config.ReadConfigFile(configPath)
	if err != nil {
		msg := `Could not open tsuru config file at %s (%s).
  For an example, see: tsuru/etc/tsuru.conf
  Note that you can specify a different config file with the --config option -- e.g.: --config=./etc/tsuru.conf
`
		fmt.Fprintf(context.Stderr, msg, configPath, err)
		return err
	}
	fmt.Fprintf(context.Stderr, "Done reading config file: %s\n", configPath)
	err = api.InitializeDBServices()
	if err != nil {
		return errors.Wrap(err, "error initializing services")
	}
	return c.Command.Run(context)
}
