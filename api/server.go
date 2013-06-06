// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/bmizerany/pat"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"net"
	"net/http"
	"os"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	log.Fatal(err)
}

func loadConfig(flags map[string]interface{}) string {
	configFile, ok := flags["config"].(string)
	if !ok {
		configFile = "/etc/tsuru/tsuru.conf"
	}
	err := config.ReadAndWatchConfigFile(configFile)
	if err != nil {
		fatal(err)
	}
	return configFile
}

func RunServer(flags map[string]interface{}) {
	configFile := loadConfig(flags)
	log.Init()
	dry, ok := flags["dry"].(bool)
	if !ok {
		dry = false
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

	m.Get("/schema/app", authorizationRequiredHandler(appSchema))
	m.Get("/schema/service", authorizationRequiredHandler(serviceSchema))
	m.Get("/schema/services", authorizationRequiredHandler(servicesSchema))

	m.Get("/quota/:owner", authorizationRequiredHandler(quotaByOwner))

	m.Get("/services/instances", authorizationRequiredHandler(serviceInstances))
	m.Get("/services/instances/:name", authorizationRequiredHandler(serviceInstance))
	m.Del("/services/instances/:name", authorizationRequiredHandler(removeServiceInstance))
	m.Post("/services/instances", authorizationRequiredHandler(createServiceInstance))
	m.Put("/services/instances/:instance/:app", authorizationRequiredHandler(bindServiceInstance))
	m.Del("/services/instances/:instance/:app", authorizationRequiredHandler(unbindServiceInstance))
	m.Get("/services/instances/:instance/status", authorizationRequiredHandler(serviceInstanceStatus))

	m.Get("/services", authorizationRequiredHandler(serviceList))
	m.Post("/services", authorizationRequiredHandler(serviceCreate))
	m.Put("/services", authorizationRequiredHandler(serviceUpdate))
	m.Del("/services/:name", authorizationRequiredHandler(serviceDelete))
	m.Get("/services/:name", authorizationRequiredHandler(serviceInfo))
	m.Get("/services/:name/doc", authorizationRequiredHandler(serviceDoc))
	m.Put("/services/:name/doc", authorizationRequiredHandler(serviceAddDoc))
	m.Put("/services/:service/:team", authorizationRequiredHandler(grantServiceAccess))
	m.Del("/services/:service/:team", authorizationRequiredHandler(revokeServiceAccess))

	m.Del("/apps/:app", authorizationRequiredHandler(appDelete))
	m.Get("/apps/:app", authorizationRequiredHandler(appInfo))
	m.Post("/apps/:app/cname", authorizationRequiredHandler(setCName))
	m.Del("/apps/:app/cname", authorizationRequiredHandler(unsetCName))
	m.Post("/apps/:app/run", authorizationRequiredHandler(runCommand))
	m.Get("/apps/:app/restart", authorizationRequiredHandler(restart))
	m.Get("/apps/:app/env", authorizationRequiredHandler(getEnv))
	m.Post("/apps/:app/env", authorizationRequiredHandler(setEnv))
	m.Del("/apps/:app/env", authorizationRequiredHandler(unsetEnv))
	m.Get("/apps", authorizationRequiredHandler(appList))
	m.Post("/apps", authorizationRequiredHandler(createApp))
	m.Put("/apps/:app/units", authorizationRequiredHandler(addUnits))
	m.Del("/apps/:app/units", authorizationRequiredHandler(removeUnits))
	m.Put("/apps/:app/:team", authorizationRequiredHandler(grantAppAccess))
	m.Del("/apps/:app/:team", authorizationRequiredHandler(revokeAppAccess))
	m.Get("/apps/:app/log", authorizationRequiredHandler(appLog))
	m.Post("/apps/:app/log", authorizationRequiredHandler(addLog))

	m.Get("/platforms", authorizationRequiredHandler(platformList))

	// These handlers don't use :app on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Get("/apps/:appname/available", authorizationRequiredHandler(appIsAvailable))
	m.Post("/apps/:appname/repository/clone", authorizationRequiredHandler(cloneRepository))

	if registrationEnabled, _ := config.GetBool("auth:user-registration"); registrationEnabled {
		m.Post("/users", handler(createUser))
	}

	m.Post("/users/:email/password", handler(resetPassword))
	m.Post("/users/:email/tokens", handler(login))
	m.Del("/users/tokens", authorizationRequiredHandler(logout))
	m.Put("/users/password", authorizationRequiredHandler(changePassword))
	m.Del("/users", authorizationRequiredHandler(removeUser))
	m.Post("/users/keys", authorizationRequiredHandler(addKeyToUser))
	m.Del("/users/keys", authorizationRequiredHandler(removeKeyFromUser))

	m.Post("/tokens", adminRequiredHandler(generateAppToken))

	m.Get("/teams", authorizationRequiredHandler(teamList))
	m.Post("/teams", authorizationRequiredHandler(createTeam))
	m.Get("/teams/:name", authorizationRequiredHandler(getTeam))
	m.Del("/teams/:name", authorizationRequiredHandler(removeTeam))
	m.Put("/teams/:team/:user", authorizationRequiredHandler(addUserToTeam))
	m.Del("/teams/:team/:user", authorizationRequiredHandler(removeUserFromTeam))

	m.Get("/healers", authorizationRequiredHandler(healers))
	m.Get("/healers/:healer", authorizationRequiredHandler(healer))

	if !dry {
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
			certFile, err := config.GetString("tls:cert-file")
			if err != nil {
				fatal(err)
			}
			keyFile, err := config.GetString("tls:key-file")
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
