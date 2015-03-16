// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/migration"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker"
	"launchpad.net/gnuflag"
)

func getProvisioner() (string, error) {
	provisioner, err := config.GetString("provisioner")
	if provisioner == "" {
		provisioner = "docker"
	}
	return provisioner, err
}

type migrateCmd struct {
	fs  *gnuflag.FlagSet
	dry bool
}

func (*migrateCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "migrate",
		Usage: "migrate",
		Desc:  "Runs migrations from previous versions of tsr",
	}
}

func (c *migrateCmd) Run(context *cmd.Context, client *cmd.Client) error {
	err := migration.Register("migrate-docker-images", c.migrateImages)
	if err != nil {
		return err
	}
	return migration.Run(context.Stdout, c.dry)
}

func (c *migrateCmd) migrateImages() error {
	provisioner, _ := getProvisioner()
	if provisioner == "docker" {
		p, err := provision.Get(provisioner)
		if err != nil {
			return err
		}
		err = p.(provision.InitializableProvisioner).Initialize()
		if err != nil {
			return err
		}
		return docker.MigrateImages()
	}
	return nil
}

func (c *migrateCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("migrate", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.dry, "dry", false, "Do not run migrations, just print what would run")
		c.fs.BoolVar(&c.dry, "n", false, "Do not run migrations, just print what would run")
	}
	return c.fs
}
