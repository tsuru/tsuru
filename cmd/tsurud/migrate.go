// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/tsuru/config"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tablecli"
	appImageMigrate "github.com/tsuru/tsuru/app/image/migrate"
	appMigrate "github.com/tsuru/tsuru/app/migrate"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/migration"
	"github.com/tsuru/tsuru/provision"
	kubeMigrate "github.com/tsuru/tsuru/provision/kubernetes/migrate"
)

const (
	nativeSchemeName       = "native"
	defaultProvisionerName = "docker"
)

func init() {
	err := migration.Register("migrate-docker-images", migrateImages)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-app-plan-router-to-app-router", appMigrate.MigrateAppPlanRouterToRouter)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-app-service-envs", appMigrate.MigrateAppTsuruServicesVarToServiceEnvs)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-app-plan-id-to-plan-name", appMigrate.MigrateAppPlanIDToPlanName)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-apps-kubernetes-crd", kubeMigrate.MigrateAppsCRDs)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-app-image-exposed-ports", appImageMigrate.MigrateExposedPorts)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
}

func getProvisioner() (string, error) {
	provisioner, err := config.GetString("provisioner")
	if provisioner == "" {
		provisioner = defaultProvisionerName
	}
	return provisioner, err
}

type migrationListCmd struct{}

func (*migrationListCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "migrate-list",
		Usage: "migrate-list",
		Desc:  "List available migration scripts from previous versions of tsurud",
	}
}

func (*migrationListCmd) Run(context *cmd.Context) error {
	migrations, err := migration.List()
	if err != nil {
		return err
	}
	tbl := tablecli.NewTable()
	tbl.Headers = tablecli.Row{"Name", "Mandatory?", "Executed?"}
	for _, m := range migrations {
		tbl.AddRow(tablecli.Row{m.Name, strconv.FormatBool(!m.Optional), strconv.FormatBool(m.Ran)})
	}
	fmt.Fprint(context.Stdout, tbl.String())
	return nil
}

type migrateCmd struct {
	fs    *gnuflag.FlagSet
	dry   bool
	force bool
	name  string
}

func (*migrateCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "migrate",
		Usage: "migrate [-n/--dry] [-f/--force] [--name name]",
		Desc: `Runs migrations from previous versions of tsurud. Only mandatory migrations
will be executed by default. To execute an optional migration the --name flag
must be informed.`,
	}
}

func (c *migrateCmd) Run(context *cmd.Context) error {
	return migration.Run(migration.RunArgs{
		Writer: context.Stdout,
		Dry:    c.dry,
		Name:   c.name,
		Force:  c.force,
	})
}

func (c *migrateCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("migrate", gnuflag.ExitOnError)
		dryMsg := "Do not run migrations, just print what would run"
		c.fs.BoolVar(&c.dry, "dry", false, dryMsg)
		c.fs.BoolVar(&c.dry, "n", false, dryMsg)
		forceMsg := "Force the execution of an already executed optional migration"
		c.fs.BoolVar(&c.force, "force", false, forceMsg)
		c.fs.BoolVar(&c.force, "f", false, forceMsg)
		c.fs.StringVar(&c.name, "name", "", "The name of an optional migration to run")
	}
	return c.fs
}

func migrateImages() error {
	provisioner, _ := getProvisioner()
	if provisioner == defaultProvisionerName {
		p, err := provision.Get(provisioner)
		if err != nil {
			return err
		}
		err = p.(provision.InitializableProvisioner).Initialize()
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}
