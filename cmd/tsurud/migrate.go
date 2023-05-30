// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tablecli"
	"github.com/tsuru/tsuru/app"
	appImageMigrate "github.com/tsuru/tsuru/app/image/migrate"
	appMigrate "github.com/tsuru/tsuru/app/migrate"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	evtMigrate "github.com/tsuru/tsuru/event/migrate"
	"github.com/tsuru/tsuru/migration"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	kubeMigrate "github.com/tsuru/tsuru/provision/kubernetes/migrate"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
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
	err = migration.Register("migrate-pool", migratePool)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-set-pool-to-app", setPoolToApps)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-service-proxy-actions", migrateServiceProxyActions)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-events-deploy", app.MigrateDeploysToEvents)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-rc-events", migrateRCEvents)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-router-unique", router.MigrateUniqueCollection)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-app-plan-router-to-app-router", appMigrate.MigrateAppPlanRouterToRouter)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-pool-teams-to-pool-constraints", pool.MigratePoolTeamsToPoolConstraints)
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
	err = migration.RegisterOptional("migrate-roles", migrateRoles)
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

func (*migrationListCmd) Run(context *cmd.Context, client *cmd.Client) error {
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

func (c *migrateCmd) Run(context *cmd.Context, client *cmd.Client) error {
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

func migratePool() error {
	db, err := db.Conn()
	if err != nil {
		return err
	}
	defer db.Close()
	poolColl := db.Collection("pool")
	var pools []pool.Pool
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

func setPoolToApps() error {
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

func migrateServiceProxyActions() error {
	db, err := db.Conn()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.UserActions().UpdateAll(
		bson.M{"action": "service-proxy-status"},
		bson.M{"$set": bson.M{"action": "service-instance-proxy"}},
	)
	return err
}

func createRole(name, contextType string) (permission.Role, error) {
	role, err := permission.NewRole(name, contextType, "")
	if err == permTypes.ErrRoleAlreadyExists {
		role, err = permission.FindRole(name)
	}
	return role, err
}

func migrateRoles() error {
	adminTeam, err := config.GetString("admin-team")
	if err != nil {
		return err
	}
	adminRole, err := createRole("admin", "global")
	if err != nil {
		return err
	}
	err = adminRole.AddPermissions("*")
	if err != nil {
		return err
	}
	teamMember, err := createRole("team-member", "team")
	if err != nil {
		return err
	}
	err = teamMember.AddPermissions(permission.PermApp.FullName(),
		permission.PermTeam.FullName(),
		permission.PermServiceInstance.FullName())
	if err != nil {
		return err
	}
	err = teamMember.AddEvent(permTypes.RoleEventTeamCreate.String())
	if err != nil {
		return err
	}
	teamCreator, err := createRole("team-creator", "global")
	if err != nil {
		return err
	}
	err = teamCreator.AddPermissions(permission.PermTeamCreate.FullName())
	if err != nil {
		return err
	}
	err = teamCreator.AddEvent(permTypes.RoleEventUserCreate.String())
	if err != nil {
		return err
	}
	users, err := auth.ListUsers()
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, u := range users {
		var teams []authTypes.Team
		err := conn.Collection("teams").Find(bson.M{"users": bson.M{"$in": []string{u.Email}}}).All(&teams)
		if err != nil {
			return err
		}
		for _, team := range teams {
			if team.Name == adminTeam {
				err := u.AddRole(adminRole.Name, "")
				if err != nil {
					fmt.Printf("%s\n", err.Error())
				}
				continue
			}
			err := u.AddRole(teamMember.Name, team.Name)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
			}
			err = u.AddRole(teamCreator.Name, "")
			if err != nil {
				fmt.Printf("%s\n", err.Error())
			}
		}
	}
	return nil
}

func migrateRCEvents() error {
	err := provision.InitializeAll()
	if err != nil {
		return err
	}
	return evtMigrate.MigrateRCEvents()
}
