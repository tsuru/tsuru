// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gnuflag"
)

type apiCmd struct {
	fs        *gnuflag.FlagSet
	dry       bool
	checkOnly bool
}

func (c *apiCmd) Run(context *cmd.Context, client *cmd.Client) error {
	log.Printf("Opening config file: %s\n", configPath)
	err := config.ReadConfigFile(configPath)
	if err != nil {
		msg := `Could not open tsuru config file at %s (%s).
  For an example, see: tsuru/etc/tsuru.conf
  Note that you can specify a different config file with the --config option -- e.g.: --config=./etc/tsuru.conf`
		log.Fatalf(msg, configPath, err)
		os.Exit(1)
	}
	fmt.Fprintf(context.Stdout, "Done reading config file: %s\n", configPath)
	err = config.Check([]config.Checker{CheckProvisioner, CheckBeanstalkd, CheckBasicConfig})
	if err != nil {
		return err
	}
	if c.checkOnly {
		return nil
	}
	api.RunServer(c.dry)
	return nil
}

func (apiCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api",
		Usage:   "api",
		Desc:    "Starts the tsuru api webserver.",
		MinArgs: 0,
	}
}

func (c *apiCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("api", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.dry, "dry", false, "dry-run: does not start the server (for testing purpose)")
		c.fs.BoolVar(&c.dry, "d", false, "dry-run: does not start the server (for testing purpose)")
		c.fs.BoolVar(&c.checkOnly, "t", false, "check only config: test your tsuru.conf file before starts.")
	}
	return c.fs
}
