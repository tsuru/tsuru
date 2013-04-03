// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	_ "github.com/globocom/tsuru/provision/juju"
	_ "github.com/globocom/tsuru/provision/local"
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
	err = config.ReadAndWatchConfigFile(*configFile)
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
	fmt.Printf("Using the database %q from the server %q.\n\n", dbName, connString)

	m := pat.New()

	m.Get("/services/instances", AuthorizationRequiredHandler(ServicesInstancesHandler))
	m.Post("/services/instances", AuthorizationRequiredHandler(CreateInstanceHandler))
	m.Put("/services/instances/:instance/:app", AuthorizationRequiredHandler(BindHandler))
	m.Del("/services/instances/:instance/:app", AuthorizationRequiredHandler(UnbindHandler))
	m.Del("/services/c/instances/:name", AuthorizationRequiredHandler(RemoveServiceInstanceHandler))
	m.Get("/services/instances/:instance/status", AuthorizationRequiredHandler(ServiceInstanceStatusHandler))

	m.Get("/services", AuthorizationRequiredHandler(ServicesHandler))
	m.Post("/services", AuthorizationRequiredHandler(CreateHandler))
	m.Put("/services", AuthorizationRequiredHandler(UpdateHandler))
	m.Del("/services/:name", AuthorizationRequiredHandler(DeleteHandler))
	m.Get("/services/:name", AuthorizationRequiredHandler(ServiceInfoHandler))
	m.Get("/services/c/:name/doc", AuthorizationRequiredHandler(Doc))
	m.Get("/services/:name/doc", AuthorizationRequiredHandler(GetDocHandler))
	m.Put("/services/:name/doc", AuthorizationRequiredHandler(AddDocHandler))
	m.Put("/services/:service/:team", AuthorizationRequiredHandler(GrantServiceAccessToTeamHandler))
	m.Del("/services/:service/:team", AuthorizationRequiredHandler(RevokeServiceAccessFromTeamHandler))

	m.Del("/apps/:name", AuthorizationRequiredHandler(appDelete))
	m.Get("/apps/:name/repository/clone", Handler(CloneRepositoryHandler))
	m.Get("/apps/:name/avaliable", Handler(AppIsAvailableHandler))
	m.Get("/apps/:name", AuthorizationRequiredHandler(AppInfo))
	m.Post("/apps/:name", AuthorizationRequiredHandler(setCName))
	m.Post("/apps/:name/run", AuthorizationRequiredHandler(RunCommand))
	m.Get("/apps/:name/restart", AuthorizationRequiredHandler(RestartHandler))
	m.Get("/apps/:name/env", AuthorizationRequiredHandler(GetEnv))
	m.Post("/apps/:name/env", AuthorizationRequiredHandler(setEnv))
	m.Del("/apps/:name/env", AuthorizationRequiredHandler(UnsetEnv))
	m.Get("/apps", AuthorizationRequiredHandler(AppList))
	m.Post("/apps", AuthorizationRequiredHandler(CreateAppHandler))
	m.Put("/apps/:name/units", AuthorizationRequiredHandler(AddUnitsHandler))
	m.Del("/apps/:name/units", AuthorizationRequiredHandler(RemoveUnitsHandler))
	m.Put("/apps/:app/:team", AuthorizationRequiredHandler(GrantAccessToTeamHandler))
	m.Del("/apps/:app/:team", AuthorizationRequiredHandler(RevokeAccessFromTeamHandler))
	m.Get("/apps/:name/log", AuthorizationRequiredHandler(appLog))
	m.Post("/apps/:name/log", Handler(AddLogHandler))

	m.Post("/users", Handler(CreateUser))
	m.Post("/users/:email/tokens", Handler(login))
	m.Put("/users/password", AuthorizationRequiredHandler(ChangePassword))
	m.Del("/users", AuthorizationRequiredHandler(RemoveUser))
	m.Post("/users/keys", AuthorizationRequiredHandler(AddKeyToUser))
	m.Del("/users/keys", AuthorizationRequiredHandler(RemoveKeyFromUser))

	m.Get("/teams", AuthorizationRequiredHandler(ListTeams))
	m.Post("/teams", AuthorizationRequiredHandler(CreateTeam))
	m.Del("/teams/:name", AuthorizationRequiredHandler(RemoveTeam))
	m.Put("/teams/:team/:user", AuthorizationRequiredHandler(AddUserToTeam))
	m.Del("/teams/:team/:user", AuthorizationRequiredHandler(RemoveUserFromTeam))

	m.Get("/healers", Handler(healers))
	m.Get("/healers/:healer", Handler(healer))

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
		tls, _ := config.GetBool("use-tls")
		if tls {
			certFile, err := config.GetString("tls-cert-file")
			if err != nil {
				fatal(err)
			}
			keyFile, err := config.GetString("tls-key-file")
			if err != nil {
				fatal(err)
			}
			fmt.Printf("tsuru HTTP/TLS server listening at %s...\n", listen)
			fatal(http.ListenAndServeTLS(listen, certFile, keyFile, m))
		} else {
			fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
			fatal(http.ListenAndServe(listen, m))
		}
	}
}
