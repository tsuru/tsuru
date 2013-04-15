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
	"net"
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

	m.Get("/services/instances", authorizationRequiredHandler(ServicesInstancesHandler))
	m.Post("/services/instances", authorizationRequiredHandler(CreateInstanceHandler))
	m.Put("/services/instances/:instance/:app", authorizationRequiredHandler(bindServiceInstance))
	m.Del("/services/instances/:instance/:app", authorizationRequiredHandler(unbindServiceInstance))
	m.Del("/services/c/instances/:name", authorizationRequiredHandler(RemoveServiceInstanceHandler))
	m.Get("/services/instances/:instance/status", authorizationRequiredHandler(ServiceInstanceStatusHandler))

	m.Get("/services", authorizationRequiredHandler(ServicesHandler))
	m.Post("/services", authorizationRequiredHandler(CreateHandler))
	m.Put("/services", authorizationRequiredHandler(UpdateHandler))
	m.Del("/services/:name", authorizationRequiredHandler(DeleteHandler))
	m.Get("/services/:name", authorizationRequiredHandler(ServiceInfoHandler))
	m.Get("/services/c/:name/doc", authorizationRequiredHandler(Doc))
	m.Get("/services/:name/doc", authorizationRequiredHandler(GetDocHandler))
	m.Put("/services/:name/doc", authorizationRequiredHandler(AddDocHandler))
	m.Put("/services/:service/:team", authorizationRequiredHandler(GrantServiceAccessToTeamHandler))
	m.Del("/services/:service/:team", authorizationRequiredHandler(RevokeServiceAccessFromTeamHandler))

	m.Del("/apps/:app", authorizationRequiredHandler(appDelete))
	m.Get("/apps/:app", authorizationRequiredHandler(appInfo))
	m.Post("/apps/:app", authorizationRequiredHandler(setCName))
	m.Post("/apps/:app/run", authorizationRequiredHandler(runCommand))
	m.Get("/apps/:app/restart", authorizationRequiredHandler(restart))
	m.Get("/apps/:app/env", authorizationRequiredHandler(getEnv))
	m.Post("/apps/:app/env", authorizationRequiredHandler(setEnv))
	m.Del("/apps/:app/env", authorizationRequiredHandler(unsetEnv))
	m.Get("/apps", authorizationRequiredHandler(appList))
	m.Post("/apps", authorizationRequiredHandler(createApp))
	m.Put("/apps/:app/units", authorizationRequiredHandler(addUnits))
	m.Del("/apps/:app/units", authorizationRequiredHandler(removeUnits))
	m.Put("/apps/:app/:team", authorizationRequiredHandler(grantAccessToTeam))
	m.Del("/apps/:app/:team", authorizationRequiredHandler(revokeAccessFromTeam))
	m.Get("/apps/:app/log", authorizationRequiredHandler(appLog))
	m.Post("/apps/:app/log", authorizationRequiredHandler(addLog))

	// These handlers don't use :app on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Get("/apps/:appname/avaliable", authorizationRequiredHandler(appIsAvailable))
	m.Get("/apps/:appname/repository/clone", authorizationRequiredHandler(cloneRepository))

	if registrationEnabled, _ := config.GetBool("auth:user-registration"); registrationEnabled {
		m.Post("/users", handler(CreateUser))
	}

	m.Post("/users/:email/tokens", handler(login))
	m.Put("/users/password", authorizationRequiredHandler(ChangePassword))
	m.Del("/users", authorizationRequiredHandler(RemoveUser))
	m.Post("/users/keys", authorizationRequiredHandler(AddKeyToUser))
	m.Del("/users/keys", authorizationRequiredHandler(RemoveKeyFromUser))

	m.Post("/tokens", adminRequiredHandler(generateAppToken))

	m.Get("/teams", authorizationRequiredHandler(ListTeams))
	m.Post("/teams", authorizationRequiredHandler(CreateTeam))
	m.Del("/teams/:name", authorizationRequiredHandler(RemoveTeam))
	m.Put("/teams/:team/:user", authorizationRequiredHandler(AddUserToTeam))
	m.Del("/teams/:team/:user", authorizationRequiredHandler(RemoveUserFromTeam))

	m.Get("/healers", authorizationRequiredHandler(healers))
	m.Get("/healers/:healer", authorizationRequiredHandler(healer))

	if !*dry {
		provisioner, err := config.GetString("provisioner")
		if err != nil {
			fmt.Printf("Warning: %q didn't declare a provisioner, using default provisioner.\n", *configFile)
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
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				fatal(err)
			}
			fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
			http.Handle("/", m)
			fatal(http.Serve(listener, nil))
		}
	}
}
