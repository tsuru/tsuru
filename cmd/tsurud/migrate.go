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
	"github.com/tsuru/tsuru/app"
	appMigrate "github.com/tsuru/tsuru/app/migrate"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	evtMigrate "github.com/tsuru/tsuru/event/migrate"
	"github.com/tsuru/tsuru/migration"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/types"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
	err = migration.Register("migrate-bs-envs", migrateBSEnvs)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-events-deploy", app.MigrateDeploysToEvents)
	if err != nil {
		log.Fatalf("unable to register migration: %s", err)
	}
	err = migration.Register("migrate-events-healer", healer.MigrateHealingToEvents)
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
	tbl := cmd.NewTable()
	tbl.Headers = cmd.Row{"Name", "Mandatory?", "Executed?"}
	for _, m := range migrations {
		tbl.AddRow(cmd.Row{m.Name, strconv.FormatBool(!m.Optional), strconv.FormatBool(m.Ran)})
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
		return docker.MigrateImages()
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
	if err != nil {
		return err
	}
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
	if err == permission.ErrRoleAlreadyExists {
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
	err = teamMember.AddEvent(permission.RoleEventTeamCreate.String())
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
	err = teamCreator.AddEvent(permission.RoleEventUserCreate.String())
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
		var teams []types.Team
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

func migrateBSEnvs() error {
	scheme, err := config.GetString("auth:scheme")
	if err != nil {
		scheme = nativeSchemeName
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	_, err = nodecontainer.InitializeBS(app.AuthScheme, app.InternalAppName)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var entry map[string]interface{}
	err = conn.Collection("bsconfig").FindId("bs").One(&entry)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return err
	}
	image, _ := entry["image"].(string)
	envs, _ := entry["envs"].([]interface{})
	var baseEnvs []string
	for _, envEntry := range envs {
		mapEntry, _ := envEntry.(map[string]interface{})
		if mapEntry == nil {
			continue
		}
		name, _ := mapEntry["name"].(string)
		value, _ := mapEntry["value"].(string)
		baseEnvs = append(baseEnvs, fmt.Sprintf("%s=%s", name, value))
	}
	bsNodeContainer, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	if err != nil {
		return err
	}
	if len(baseEnvs) > 0 {
		bsNodeContainer.Config.Env = append(bsNodeContainer.Config.Env, baseEnvs...)
	}
	bsNodeContainer.PinnedImage = image
	err = nodecontainer.AddNewContainer("", bsNodeContainer)
	if err != nil {
		return err
	}
	pools, _ := entry["pools"].([]interface{})
	for _, poolData := range pools {
		poolMap, _ := poolData.(map[string]interface{})
		if poolMap == nil {
			continue
		}
		poolName, _ := poolMap["name"].(string)
		if poolName == "" {
			continue
		}
		envs, _ := poolMap["envs"].([]interface{})
		var toAdd []string
		for _, envEntry := range envs {
			mapEntry, _ := envEntry.(map[string]interface{})
			if mapEntry == nil {
				continue
			}
			name, _ := mapEntry["name"].(string)
			value, _ := mapEntry["value"].(string)
			toAdd = append(toAdd, fmt.Sprintf("%s=%s", name, value))
		}
		if len(toAdd) > 0 {
			bsCont := nodecontainer.NodeContainerConfig{Name: nodecontainer.BsDefaultName}
			bsCont.Config.Env = append(bsCont.Config.Env, toAdd...)
			err = nodecontainer.AddNewContainer(poolName, &bsCont)
			if err != nil {
				return err
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
