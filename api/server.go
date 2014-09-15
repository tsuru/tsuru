// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/codegangsta/negroni"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

type TsuruHandler struct {
	method string
	path   string
	h      http.Handler
}

func fatal(err error) {
	log.Fatal(err.Error())
}

var tsuruHandlerList []TsuruHandler

//RegisterHandler inserts a handler on a list of handlers
func RegisterHandler(path string, method string, h http.Handler) {
	var th TsuruHandler
	th.path = path
	th.method = method
	th.h = h
	tsuruHandlerList = append(tsuruHandlerList, th)
}

func getAuthScheme() (string, error) {
	name, err := config.GetString("auth:scheme")
	if name == "" {
		name = "native"
	}
	return name, err
}

// RunServer starts tsuru API server. The dry parameter indicates whether the
// server should run in dry mode, not starting the HTTP listener (for testing
// purposes).
func RunServer(dry bool) http.Handler {
	log.Init()
	connString, err := config.GetString("database:url")
	if err != nil {
		connString = db.DefaultDatabaseURL
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		dbName = db.DefaultDatabaseName
	}
	fmt.Printf("Using the database %q from the server %q.\n\n", dbName, connString)

	m := &delayedRouter{}

	for _, handler := range tsuruHandlerList {
		m.Add(handler.method, handler.path, handler.h)
	}

	m.Add("Get", "/schema/app", authorizationRequiredHandler(appSchema))
	m.Add("Get", "/schema/service", authorizationRequiredHandler(serviceSchema))
	m.Add("Get", "/schema/services", authorizationRequiredHandler(servicesSchema))

	m.Add("Get", "/services/instances", authorizationRequiredHandler(serviceInstances))
	m.Add("Get", "/services/instances/{name}", authorizationRequiredHandler(serviceInstance))
	m.Add("Delete", "/services/instances/{name}", authorizationRequiredHandler(removeServiceInstance))
	m.Add("Post", "/services/instances", authorizationRequiredHandler(createServiceInstance))
	m.Add("Put", "/services/instances/{instance}/{app}", authorizationRequiredHandler(bindServiceInstance))
	m.Add("Delete", "/services/instances/{instance}/{app}", authorizationRequiredHandler(unbindServiceInstance))
	m.Add("Get", "/services/instances/{instance}/status", authorizationRequiredHandler(serviceInstanceStatus))

	m.AddAll("/services/proxy/{instance}", authorizationRequiredHandler(serviceProxy))

	m.Add("Get", "/services", authorizationRequiredHandler(serviceList))
	m.Add("Post", "/services", authorizationRequiredHandler(serviceCreate))
	m.Add("Put", "/services", authorizationRequiredHandler(serviceUpdate))
	m.Add("Delete", "/services/{name}", authorizationRequiredHandler(serviceDelete))
	m.Add("Get", "/services/{name}", authorizationRequiredHandler(serviceInfo))
	m.Add("Get", "/services/{name}/plans", authorizationRequiredHandler(servicePlans))
	m.Add("Get", "/services/{name}/doc", authorizationRequiredHandler(serviceDoc))
	m.Add("Put", "/services/{name}/doc", authorizationRequiredHandler(serviceAddDoc))
	m.Add("Put", "/services/{service}/{team}", authorizationRequiredHandler(grantServiceAccess))
	m.Add("Delete", "/services/{service}/{team}", authorizationRequiredHandler(revokeServiceAccess))

	m.Add("Delete", "/apps/{app}", authorizationRequiredHandler(appDelete))
	m.Add("Get", "/apps/{app}", authorizationRequiredHandler(appInfo))
	m.Add("Post", "/apps/{app}/cname", authorizationRequiredHandler(setCName))
	m.Add("Delete", "/apps/{app}/cname", authorizationRequiredHandler(unsetCName))
	runHandler := authorizationRequiredHandler(runCommand)
	m.Add("Post", "/apps/{app}/run", runHandler)
	m.Add("Post", "/apps/{app}/restart", authorizationRequiredHandler(restart))
	m.Add("Post", "/apps/{app}/start", authorizationRequiredHandler(start))
	m.Add("Post", "/apps/{app}/stop", authorizationRequiredHandler(stop))
	m.Add("Get", "/apps/{appname}/quota", AdminRequiredHandler(getAppQuota))
	m.Add("Post", "/apps/{appname}/quota", AdminRequiredHandler(changeAppQuota))
	m.Add("Get", "/apps/{app}/env", authorizationRequiredHandler(getEnv))
	m.Add("Post", "/apps/{app}/env", authorizationRequiredHandler(setEnv))
	m.Add("Delete", "/apps/{app}/env", authorizationRequiredHandler(unsetEnv))
	m.Add("Get", "/apps", authorizationRequiredHandler(appList))
	m.Add("Post", "/apps", authorizationRequiredHandler(createApp))
	m.Add("Post", "/apps/{app}/team-owner", authorizationRequiredHandler(setTeamOwner))
	forceDeleteLockHandler := AdminRequiredHandler(forceDeleteLock)
	m.Add("Delete", "/apps/{app}/lock", forceDeleteLockHandler)
	m.Add("Put", "/apps/{app}/units", authorizationRequiredHandler(addUnits))
	m.Add("Delete", "/apps/{app}/units", authorizationRequiredHandler(removeUnits))
	registerUnitHandler := authorizationRequiredHandler(registerUnit)
	m.Add("Post", "/apps/{app}/units/register", registerUnitHandler)
	m.Add("Post", "/apps/{app}/units/{unit}", authorizationRequiredHandler(setUnitStatus))
	m.Add("Put", "/apps/{app}/teams/{team}", authorizationRequiredHandler(grantAppAccess))
	m.Add("Delete", "/apps/{app}/teams/{team}", authorizationRequiredHandler(revokeAppAccess))
	m.Add("Get", "/apps/{app}/log", authorizationRequiredHandler(appLog))
	logPostHandler := authorizationRequiredHandler(addLog)
	m.Add("Post", "/apps/{app}/log", logPostHandler)

	m.Add("Get", "/deploys", AdminRequiredHandler(deploysList))
	m.Add("Get", "/deploys/{deploy}", AdminRequiredHandler(deployInfo))

	m.Add("Get", "/platforms", authorizationRequiredHandler(platformList))
	m.Add("Post", "/platforms", AdminRequiredHandler(platformAdd))
	m.Add("Put", "/platforms/{name}", AdminRequiredHandler(platformUpdate))
	m.Add("Delete", "/platforms/{name}", AdminRequiredHandler(platformRemove))

	// These handlers don't use :app on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Add("Get", "/apps/{appname}/available", authorizationRequiredHandler(appIsAvailable))
	m.Add("Post", "/apps/{appname}/repository/clone", authorizationRequiredHandler(deploy))
	m.Add("Post", "/apps/{appname}/deploy", authorizationRequiredHandler(deploy))

	m.Add("Post", "/users", Handler(createUser))
	m.Add("Get", "/auth/scheme", Handler(authScheme))
	m.Add("Post", "/auth/login", Handler(login))
	m.Add("Post", "/users/{email}/password", Handler(resetPassword))
	m.Add("Post", "/users/{email}/tokens", Handler(login))
	m.Add("Get", "/users/{email}/quota", AdminRequiredHandler(getUserQuota))
	m.Add("Post", "/users/{email}/quota", AdminRequiredHandler(changeUserQuota))
	m.Add("Delete", "/users/tokens", authorizationRequiredHandler(logout))
	m.Add("Put", "/users/password", authorizationRequiredHandler(changePassword))
	m.Add("Delete", "/users", authorizationRequiredHandler(removeUser))
	m.Add("Get", "/users/{email}/keys", authorizationRequiredHandler(listKeys))
	m.Add("Post", "/users/keys", authorizationRequiredHandler(addKeyToUser))
	m.Add("Delete", "/users/keys", authorizationRequiredHandler(removeKeyFromUser))

	m.Add("Post", "/tokens", AdminRequiredHandler(generateAppToken))

	m.Add("Delete", "/logs", AdminRequiredHandler(logRemove))

	m.Add("Get", "/teams", authorizationRequiredHandler(teamList))
	m.Add("Post", "/teams", authorizationRequiredHandler(createTeam))
	m.Add("Get", "/teams/{name}", authorizationRequiredHandler(getTeam))
	m.Add("Delete", "/teams/{name}", authorizationRequiredHandler(removeTeam))
	m.Add("Put", "/teams/{team}/{user}", authorizationRequiredHandler(addUserToTeam))
	m.Add("Delete", "/teams/{team}/{user}", authorizationRequiredHandler(removeUserFromTeam))

	m.Add("Get", "/healers", authorizationRequiredHandler(healers))
	m.Add("Get", "/healers/{healer}", authorizationRequiredHandler(healer))

	m.Add("Put", "/swap", authorizationRequiredHandler(swap))

	m.Add("Get", "/healthcheck/", http.HandlerFunc(healthcheck))

	m.Add("Get", "/iaas/machines", AdminRequiredHandler(machinesList))
	m.Add("Delete", "/iaas/machines/{machine_id}", AdminRequiredHandler(machineDestroy))

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(negroni.NewLogger())
	n.UseHandler(m)
	n.Use(negroni.HandlerFunc(contextClearerMiddleware))
	n.Use(negroni.HandlerFunc(flushingWriterMiddleware))
	n.Use(negroni.HandlerFunc(errorHandlingMiddleware))
	n.Use(negroni.HandlerFunc(setVersionHeadersMiddleware))
	n.Use(negroni.HandlerFunc(authTokenMiddleware))
	n.Use(&appLockMiddleware{excludedHandlers: []http.Handler{
		logPostHandler,
		runHandler,
		forceDeleteLockHandler,
		registerUnitHandler,
	}})
	n.UseHandler(http.HandlerFunc(runDelayedHandler))

	if !dry {
		provisioner, err := getProvisioner()
		if err != nil {
			fmt.Printf("Warning: configuration didn't declare a provisioner, using default provisioner.\n")
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n\n", provisioner)
		scheme, err := getAuthScheme()
		if err != nil {
			fmt.Printf("Warning: configuration didn't declare a auth:scheme, using default scheme.\n")
		}
		app.AuthScheme, err = auth.GetScheme(scheme)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q auth scheme.\n\n", scheme)
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
			fatal(http.ListenAndServeTLS(listen, certFile, keyFile, n))
		} else {
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				fatal(err)
			}
			fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
			http.Handle("/", n)
			fatal(http.Serve(listener, nil))
		}
	}
	return n
}
