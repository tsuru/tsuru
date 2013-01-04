// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/provision/juju"
	stdlog "log"
	"log/syslog"
	"net/http"
	"os"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	log.Fatal(err)
}

func main() {
	logger, err := syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		stdlog.Fatal(err)
	}
	log.SetLogger(logger)
	configFile := flag.String("config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	dry := flag.Bool("dry", false, "dry-run: does not start the server (for testing purpose)")
	flag.Parse()
	err = config.ReadConfigFile(*configFile)
	if err != nil {
		fatal(err)
	}
	connString, err := config.GetString("database:url")
	if err != nil {
		fatal(err)
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		fatal(err)
	}
	db.Session, err = db.Open(connString, dbName)
	if err != nil {
		fatal(err)
	}
	defer db.Session.Close()
	fmt.Printf("Connected to MongoDB server at %s.\n", connString)
	fmt.Printf("Using the database %q.\n\n", dbName)

	m := pat.New()

	m.Get("/services/instances", AuthorizationRequiredHandler(api.ServicesInstancesHandler))
	m.Post("/services/instances", AuthorizationRequiredHandler(api.CreateInstanceHandler))
	m.Put("/services/instances/:instance/:app", AuthorizationRequiredHandler(api.BindHandler))
	m.Del("/services/instances/:instance/:app", AuthorizationRequiredHandler(api.UnbindHandler))
	m.Del("/services/c/instances/:name", AuthorizationRequiredHandler(api.RemoveServiceInstanceHandler))
	m.Get("/services/instances/:instance/status", AuthorizationRequiredHandler(api.ServiceInstanceStatusHandler))

	m.Get("/services", AuthorizationRequiredHandler(api.ServicesHandler))
	m.Post("/services", AuthorizationRequiredHandler(api.CreateHandler))
	m.Put("/services", AuthorizationRequiredHandler(api.UpdateHandler))
	m.Del("/services/:name", AuthorizationRequiredHandler(api.DeleteHandler))
	m.Get("/services/:name", AuthorizationRequiredHandler(api.ServiceInfoHandler))
	m.Get("/services/c/:name/doc", AuthorizationRequiredHandler(api.Doc))
	m.Get("/services/:name/doc", AuthorizationRequiredHandler(api.GetDocHandler))
	m.Put("/services/:name/doc", AuthorizationRequiredHandler(api.AddDocHandler))
	m.Put("/services/:service/:team", AuthorizationRequiredHandler(api.GrantAccessToTeamHandler))
	m.Del("/services/:service/:team", AuthorizationRequiredHandler(api.RevokeAccessFromTeamHandler))

	m.Del("/apps/:name", AuthorizationRequiredHandler(api.AppDelete))
	m.Get("/apps/:name/repository/clone", Handler(api.CloneRepositoryHandler))
	m.Get("/apps/:name/avaliable", Handler(api.AppIsAvaliableHandler))
	m.Get("/apps/:name", AuthorizationRequiredHandler(api.AppInfo))
	m.Post("/apps/:name/run", AuthorizationRequiredHandler(api.RunCommand))
	m.Get("/apps/:name/restart", AuthorizationRequiredHandler(api.RestartHandler))
	m.Get("/apps/:name/env", AuthorizationRequiredHandler(api.GetEnv))
	m.Post("/apps/:name/env", AuthorizationRequiredHandler(api.SetEnv))
	m.Del("/apps/:name/env", AuthorizationRequiredHandler(api.UnsetEnv))
	m.Get("/apps", AuthorizationRequiredHandler(api.AppList))
	m.Post("/apps", AuthorizationRequiredHandler(api.CreateAppHandler))
	m.Put("/apps/:name/units", AuthorizationRequiredHandler(api.AddUnitsHandler))
	m.Del("/apps/:name/units", AuthorizationRequiredHandler(api.RemoveUnitsHandler))
	m.Put("/apps/:app/:team", AuthorizationRequiredHandler(api.GrantAccessToTeamHandler))
	m.Del("/apps/:app/:team", AuthorizationRequiredHandler(api.RevokeAccessFromTeamHandler))
	m.Get("/apps/:name/log", AuthorizationRequiredHandler(api.AppLog))
	m.Post("/apps/:name/log", Handler(api.AddLogHandler))

	m.Post("/users", Handler(auth.CreateUser))
	m.Post("/users/:email/tokens", Handler(auth.Login))
	m.Put("/users/password", AuthorizationRequiredHandler(auth.ChangePassword))
	m.Del("/users", AuthorizationRequiredHandler(auth.RemoveUser))
	m.Post("/users/keys", AuthorizationRequiredHandler(auth.AddKeyToUser))
	m.Del("/users/keys", AuthorizationRequiredHandler(auth.RemoveKeyFromUser))

	m.Get("/teams", AuthorizationRequiredHandler(auth.ListTeams))
	m.Post("/teams", AuthorizationRequiredHandler(auth.CreateTeam))
	m.Del("/teams/:name", AuthorizationRequiredHandler(auth.RemoveTeam))
	m.Put("/teams/:team/:user", AuthorizationRequiredHandler(auth.AddUserToTeam))
	m.Del("/teams/:team/:user", AuthorizationRequiredHandler(auth.RemoveUserFromTeam))

	if !*dry {
		provisioner, err := config.GetString("provisioner")
		if err != nil {
			fmt.Printf("Warning: %q didn't declare a provisioner, using default provisioner.\n", configFile)
			provisioner = "juju"
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n\n", provisioner)

		listen, err := config.GetString("listen")
		if err != nil {
			fatal(err)
		}
		fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
		fatal(http.ListenAndServe(listen, m))
	}
}
