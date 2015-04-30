// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/migration"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker"
	"gopkg.in/mgo.v2/bson"
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
	err = migration.Register("migrate-pool", c.migratePool)
	if err != nil {
		return err
	}
	err = migration.Register("migrate-set-pool-to-app", c.setPoolToApps)
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

func (c *migrateCmd) migratePool() error {
	db, err := db.Conn()
	if err != nil {
		return err
	}
	defer db.Close()
	poolColl := db.Collection("pool")
	if err != nil {
		return err
	}
	var pools []provision.Pool
	err = db.Collection("docker_scheduler").Find(nil).All(&pools)
	if err != nil {
		return err
	}
	for _, p := range pools {
		err = poolColl.Insert(p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *migrateCmd) setPoolToApps() error {
	db, err := db.Conn()
	if err != nil {
		return err
	}
	defer db.Close()
	var apps []app.App
	var tooManyPoolsApps []app.App
	err = db.Apps().Find(nil).All(&apps)
	if err != nil {
		return err
	}
	for _, a := range apps {
		err = a.SetPool()
		if err != nil {
			tooManyPoolsApps = append(tooManyPoolsApps, a)
			continue
		}
		err = db.Apps().Update(bson.M{"name": a.Name}, bson.M{"$set": bson.M{"pool": a.Pool}})
		if err != nil {
			return err
		}
	}
	if len(tooManyPoolsApps) > 0 {
		fmt.Println("Apps bellow couldn't be migrated because they are in more than one pool.")
		fmt.Println("To fix this, please run `tsuru app-change-pool <pool_name> -a app` for each app.")
		fmt.Println("*****************************************")
		for _, a := range tooManyPoolsApps {
			fmt.Println(a.Name)
		}
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
