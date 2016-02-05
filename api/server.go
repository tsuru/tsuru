// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/tsuru/config"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	_ "github.com/tsuru/tsuru/auth/saml"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"golang.org/x/net/websocket"
	"gopkg.in/tylerb/graceful.v1"
)

const Version = "0.13.0"

func getProvisioner() (string, error) {
	provisioner, err := config.GetString("provisioner")
	if provisioner == "" {
		provisioner = "docker"
	}
	return provisioner, err
}

type TsuruHandler struct {
	method string
	path   string
	h      http.Handler
}

func fatal(err error) {
	fmt.Println(err.Error())
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
	fmt.Printf("Using mongodb database %q from the server %q.\n", dbName, connString)

	m := &apiRouter.DelayedRouter{}

	for _, handler := range tsuruHandlerList {
		m.Add(handler.method, handler.path, handler.h)
	}

	if disableIndex, _ := config.GetBool("disable-index-page"); !disableIndex {
		m.Add("Get", "/", Handler(index))
	}
	m.Add("Get", "/info", Handler(info))

	m.Add("Get", "/services/instances", AuthorizationRequiredHandler(serviceInstances))
	m.Add("Get", "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(serviceInstance))
	m.Add("Delete", "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(removeServiceInstance))
	m.Add("Post", "/services/instances", AuthorizationRequiredHandler(createServiceInstance))
	m.Add("Post", "/services/{service}/instances/{instance}/update", AuthorizationRequiredHandler(updateServiceInstance))
	m.Add("Put", "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(bindServiceInstance))
	m.Add("Delete", "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(unbindServiceInstance))
	m.Add("Get", "/services/{service}/instances/{instance}/status", AuthorizationRequiredHandler(serviceInstanceStatus))
	m.Add("Put", "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceGrantTeam))
	m.Add("Delete", "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceRevokeTeam))
	m.Add("Get", "/services/{service}/instances/{instance}/info", AuthorizationRequiredHandler(serviceInstanceInfo))

	m.AddAll("/services/{service}/proxy/{instance}", AuthorizationRequiredHandler(serviceInstanceProxy))
	m.AddAll("/services/proxy/service/{service}", AuthorizationRequiredHandler(serviceProxy))

	m.Add("Get", "/services", AuthorizationRequiredHandler(serviceList))
	m.Add("Post", "/services", AuthorizationRequiredHandler(serviceCreate))
	m.Add("Put", "/services", AuthorizationRequiredHandler(serviceUpdate))
	m.Add("Delete", "/services/{name}", AuthorizationRequiredHandler(serviceDelete))
	m.Add("Get", "/services/{name}", AuthorizationRequiredHandler(serviceInfo))
	m.Add("Get", "/services/{name}/plans", AuthorizationRequiredHandler(servicePlans))
	m.Add("Get", "/services/{name}/doc", AuthorizationRequiredHandler(serviceDoc))
	m.Add("Put", "/services/{name}/doc", AuthorizationRequiredHandler(serviceAddDoc))
	m.Add("Put", "/services/{service}/team/{team}", AuthorizationRequiredHandler(grantServiceAccess))
	m.Add("Delete", "/services/{service}/team/{team}", AuthorizationRequiredHandler(revokeServiceAccess))

	m.Add("Delete", "/apps/{app}", AuthorizationRequiredHandler(appDelete))
	m.Add("Get", "/apps/{app}", AuthorizationRequiredHandler(appInfo))
	m.Add("Post", "/apps/{app}/cname", AuthorizationRequiredHandler(setCName))
	m.Add("Delete", "/apps/{app}/cname", AuthorizationRequiredHandler(unsetCName))
	runHandler := AuthorizationRequiredHandler(runCommand)
	m.Add("Post", "/apps/{app}/run", runHandler)
	m.Add("Post", "/apps/{app}/restart", AuthorizationRequiredHandler(restart))
	m.Add("Post", "/apps/{app}/start", AuthorizationRequiredHandler(start))
	m.Add("Post", "/apps/{app}/stop", AuthorizationRequiredHandler(stop))
	m.Add("Post", "/apps/{app}/sleep", AuthorizationRequiredHandler(sleep))
	m.Add("Get", "/apps/{appname}/quota", AuthorizationRequiredHandler(getAppQuota))
	m.Add("Post", "/apps/{appname}/quota", AuthorizationRequiredHandler(changeAppQuota))
	m.Add("Post", "/apps/{appname}", AuthorizationRequiredHandler(updateApp))
	m.Add("Get", "/apps/{app}/env", AuthorizationRequiredHandler(getEnv))
	m.Add("Post", "/apps/{app}/env", AuthorizationRequiredHandler(setEnv))
	m.Add("Delete", "/apps/{app}/env", AuthorizationRequiredHandler(unsetEnv))
	m.Add("Get", "/apps", AuthorizationRequiredHandler(appList))
	m.Add("Post", "/apps", AuthorizationRequiredHandler(createApp))
	forceDeleteLockHandler := AuthorizationRequiredHandler(forceDeleteLock)
	m.Add("Delete", "/apps/{app}/lock", forceDeleteLockHandler)
	m.Add("Put", "/apps/{app}/units", AuthorizationRequiredHandler(addUnits))
	m.Add("Delete", "/apps/{app}/units", AuthorizationRequiredHandler(removeUnits))
	registerUnitHandler := AuthorizationRequiredHandler(registerUnit)
	m.Add("Post", "/apps/{app}/units/register", registerUnitHandler)
	setUnitStatusHandler := AuthorizationRequiredHandler(setUnitStatus)
	m.Add("Post", "/apps/{app}/units/{unit}", setUnitStatusHandler)
	m.Add("Put", "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(grantAppAccess))
	m.Add("Delete", "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(revokeAppAccess))
	m.Add("Get", "/apps/{app}/log", AuthorizationRequiredHandler(appLog))
	logPostHandler := AuthorizationRequiredHandler(addLog)
	m.Add("Post", "/apps/{app}/log", logPostHandler)
	m.Add("Post", "/apps/{appname}/deploy/rollback", AuthorizationRequiredHandler(deployRollback))
	m.Add("Get", "/apps/{app}/metric/envs", AuthorizationRequiredHandler(appMetricEnvs))
	m.Add("Post", "/apps/{app}/routes", AuthorizationRequiredHandler(appRebuildRoutes))

	m.Add("Post", "/units/status", AuthorizationRequiredHandler(setUnitsStatus))

	m.Add("Get", "/deploys", AuthorizationRequiredHandler(deploysList))
	m.Add("Get", "/deploys/{deploy}", AuthorizationRequiredHandler(deployInfo))

	m.Add("Get", "/platforms", AuthorizationRequiredHandler(platformList))
	m.Add("Post", "/platforms", AuthorizationRequiredHandler(platformAdd))
	m.Add("Put", "/platforms/{name}", AuthorizationRequiredHandler(platformUpdate))
	m.Add("Delete", "/platforms/{name}", AuthorizationRequiredHandler(platformRemove))

	// These handlers don't use {app} on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Add("Get", "/apps/{appname}/available", AuthorizationRequiredHandler(appIsAvailable))
	m.Add("Post", "/apps/{appname}/repository/clone", AuthorizationRequiredHandler(deploy))
	m.Add("Post", "/apps/{appname}/deploy", AuthorizationRequiredHandler(deploy))
	diffDeployHandler := AuthorizationRequiredHandler(diffDeploy)
	m.Add("Post", "/apps/{appname}/diff", diffDeployHandler)

	// Shell also doesn't use {app} on purpose. Middlewares don't play well
	// with websocket.
	m.Add("Get", "/apps/{appname}/shell", websocket.Handler(remoteShellHandler))

	m.Add("Get", "/users", AuthorizationRequiredHandler(listUsers))
	m.Add("Post", "/users", Handler(createUser))
	m.Add("Get", "/users/info", AuthorizationRequiredHandler(userInfo))
	m.Add("Get", "/auth/scheme", Handler(authScheme))
	m.Add("Post", "/auth/login", Handler(login))

	m.Add("Post", "/auth/saml", Handler(samlCallbackLogin))
	m.Add("Get", "/auth/saml", Handler(samlMetadata))

	m.Add("Post", "/users/{email}/password", Handler(resetPassword))
	m.Add("Post", "/users/{email}/tokens", Handler(login))
	m.Add("Get", "/users/{email}/quota", AuthorizationRequiredHandler(getUserQuota))
	m.Add("Post", "/users/{email}/quota", AuthorizationRequiredHandler(changeUserQuota))
	m.Add("Delete", "/users/tokens", AuthorizationRequiredHandler(logout))
	m.Add("Put", "/users/password", AuthorizationRequiredHandler(changePassword))
	m.Add("Delete", "/users", AuthorizationRequiredHandler(removeUser))
	m.Add("Get", "/users/keys", AuthorizationRequiredHandler(listKeys))
	m.Add("Post", "/users/keys", AuthorizationRequiredHandler(addKeyToUser))
	m.Add("Delete", "/users/keys", AuthorizationRequiredHandler(removeKeyFromUser))
	m.Add("Get", "/users/api-key", AuthorizationRequiredHandler(showAPIToken))
	m.Add("Post", "/users/api-key", AuthorizationRequiredHandler(regenerateAPIToken))

	m.Add("Get", "/logs", websocket.Handler(addLogs))

	m.Add("Get", "/teams", AuthorizationRequiredHandler(teamList))
	m.Add("Post", "/teams", AuthorizationRequiredHandler(createTeam))
	m.Add("Delete", "/teams/{name}", AuthorizationRequiredHandler(removeTeam))

	m.Add("Put", "/swap", AuthorizationRequiredHandler(swap))

	m.Add("Get", "/healthcheck/", http.HandlerFunc(healthcheck))

	m.Add("Get", "/iaas/machines", AuthorizationRequiredHandler(machinesList))
	m.Add("Delete", "/iaas/machines/{machine_id}", AuthorizationRequiredHandler(machineDestroy))
	m.Add("Get", "/iaas/templates", AuthorizationRequiredHandler(templatesList))
	m.Add("Post", "/iaas/templates", AuthorizationRequiredHandler(templateCreate))
	m.Add("Put", "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateUpdate))
	m.Add("Delete", "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateDestroy))

	m.Add("Get", "/plans", AuthorizationRequiredHandler(listPlans))
	m.Add("Post", "/plans", AuthorizationRequiredHandler(addPlan))
	m.Add("Delete", "/plans/{planname}", AuthorizationRequiredHandler(removePlan))
	m.Add("Get", "/plans/routers", AuthorizationRequiredHandler(listRouters))

	m.Add("Get", "/pools", AuthorizationRequiredHandler(listPoolsToUser))
	m.Add("Post", "/pool", AuthorizationRequiredHandler(addPoolHandler))
	m.Add("Delete", "/pool", AuthorizationRequiredHandler(removePoolHandler))
	m.Add("Post", "/pool/{name}", AuthorizationRequiredHandler(poolUpdateHandler))
	m.Add("Post", "/pool/{name}/team", AuthorizationRequiredHandler(addTeamToPoolHandler))
	m.Add("Delete", "/pool/{name}/team", AuthorizationRequiredHandler(removeTeamToPoolHandler))

	m.Add("Get", "/roles", AuthorizationRequiredHandler(listRoles))
	m.Add("Post", "/roles", AuthorizationRequiredHandler(addRole))
	m.Add("Get", "/roles/{name}", AuthorizationRequiredHandler(roleInfo))
	m.Add("Delete", "/roles/{name}", AuthorizationRequiredHandler(removeRole))
	m.Add("Post", "/roles/{name}/permissions", AuthorizationRequiredHandler(addPermissions))
	m.Add("Delete", "/roles/{name}/permissions/{permission}", AuthorizationRequiredHandler(removePermissions))
	m.Add("Post", "/roles/{name}/user", AuthorizationRequiredHandler(assignRole))
	m.Add("Delete", "/roles/{name}/user/{email}", AuthorizationRequiredHandler(dissociateRole))
	m.Add("Get", "/role/default", AuthorizationRequiredHandler(listDefaultRoles))
	m.Add("Post", "/role/default", AuthorizationRequiredHandler(addDefaultRole))
	m.Add("Delete", "/role/default", AuthorizationRequiredHandler(removeDefaultRole))
	m.Add("Get", "/permissions", AuthorizationRequiredHandler(listPermissions))

	m.Add("Get", "/debug/goroutines", AuthorizationRequiredHandler(dumpGoroutines))
	m.Add("Get", "/debug/pprof/", AuthorizationRequiredHandler(indexHandler))
	m.Add("Get", "/debug/pprof/cmdline", AuthorizationRequiredHandler(cmdlineHandler))
	m.Add("Get", "/debug/pprof/profile", AuthorizationRequiredHandler(profileHandler))
	m.Add("Get", "/debug/pprof/symbol", AuthorizationRequiredHandler(symbolHandler))
	m.Add("Get", "/debug/pprof/heap", AuthorizationRequiredHandler(indexHandler))
	m.Add("Get", "/debug/pprof/goroutine", AuthorizationRequiredHandler(indexHandler))
	m.Add("Get", "/debug/pprof/threadcreate", AuthorizationRequiredHandler(indexHandler))
	m.Add("Get", "/debug/pprof/block", AuthorizationRequiredHandler(indexHandler))

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(newLoggerMiddleware())
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
		setUnitStatusHandler,
		diffDeployHandler,
	}})
	n.UseHandler(http.HandlerFunc(runDelayedHandler))

	if !dry {
		var startupMessage string
		routers, err := router.List()
		if err != nil {
			fatal(err)
		}
		for _, routerDesc := range routers {
			var r router.Router
			r, err = router.Get(routerDesc.Name)
			if err != nil {
				fatal(err)
			}
			fmt.Printf("Registered router %q", routerDesc.Name)
			if messageRouter, ok := r.(router.MessageRouter); ok {
				startupMessage, err = messageRouter.StartupMessage()
				if err == nil && startupMessage != "" {
					fmt.Printf(": %s", startupMessage)
				}
			}
			fmt.Println()
		}
		defaultRouter, _ := config.GetString("docker:router")
		fmt.Printf("Default router is %q.\n", defaultRouter)
		repoManager, err := config.GetString("repo-manager")
		if err != nil {
			repoManager = "gandalf"
			fmt.Println("Warning: configuration didn't declare a repository manager, using default manager.")
		}
		fmt.Printf("Using %q repository manager.\n", repoManager)
		provisioner, err := getProvisioner()
		if err != nil {
			fmt.Println("Warning: configuration didn't declare a provisioner, using default provisioner.")
		}
		app.Provisioner, err = provision.Get(provisioner)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q provisioner.\n", provisioner)
		if initializableProvisioner, ok := app.Provisioner.(provision.InitializableProvisioner); ok {
			err = initializableProvisioner.Initialize()
			if err != nil {
				fatal(err)
			}
		}
		if messageProvisioner, ok := app.Provisioner.(provision.MessageProvisioner); ok {
			startupMessage, err = messageProvisioner.StartupMessage()
			if err == nil && startupMessage != "" {
				fmt.Print(startupMessage)
			}
		}
		scheme, err := getAuthScheme()
		if err != nil {
			fmt.Printf("Warning: configuration didn't declare auth:scheme, using default scheme.\n")
		}
		app.AuthScheme, err = auth.GetScheme(scheme)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Using %q auth scheme.\n", scheme)
		fmt.Println("Checking components status:")
		results := hc.Check()
		for _, result := range results {
			if result.Status != hc.HealthCheckOK {
				fmt.Printf("    WARNING: %q is not working: %s\n", result.Name, result.Status)
			}
		}
		fmt.Println("    Components checked.")
		listen, err := config.GetString("listen")
		if err != nil {
			fatal(err)
		}
		shutdownChan := make(chan bool)
		shutdownTimeout, _ := config.GetInt("shutdown-timeout")
		if shutdownTimeout == 0 {
			shutdownTimeout = 10 * 60
		}
		idleTracker := newIdleTracker()
		shutdown.Register(idleTracker)
		shutdown.Register(&logTracker)
		readTimeout, _ := config.GetInt("server:read-timeout")
		writeTimeout, _ := config.GetInt("server:write-timeout")
		srv := &graceful.Server{
			Timeout: time.Duration(shutdownTimeout) * time.Second,
			Server: &http.Server{
				ReadTimeout:  time.Duration(readTimeout) * time.Second,
				WriteTimeout: time.Duration(writeTimeout) * time.Second,
				Addr:         listen,
				Handler:      n,
			},
			ConnState: func(conn net.Conn, state http.ConnState) {
				idleTracker.trackConn(conn, state)
			},
			ShutdownInitiated: func() {
				fmt.Println("tsuru is shutting down, waiting for pending connections to finish.")
				handlers := shutdown.All()
				wg := sync.WaitGroup{}
				for _, h := range handlers {
					wg.Add(1)
					go func(h shutdown.Shutdownable) {
						defer wg.Done()
						fmt.Printf("running shutdown handler for %v...\n", h)
						h.Shutdown()
						fmt.Printf("running shutdown handler for %v. DONE.\n", h)
					}(h)
				}
				wg.Wait()
				close(shutdownChan)
			},
		}
		tls, _ := config.GetBool("use-tls")
		if tls {
			var (
				certFile string
				keyFile  string
			)
			certFile, err = config.GetString("tls:cert-file")
			if err != nil {
				fatal(err)
			}
			keyFile, err = config.GetString("tls:key-file")
			if err != nil {
				fatal(err)
			}
			fmt.Printf("tsuru HTTP/TLS server listening at %s...\n", listen)
			err = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			fmt.Printf("tsuru HTTP server listening at %s...\n", listen)
			err = srv.ListenAndServe()
		}
		if err != nil {
			fmt.Printf("Listening stopped: %s\n", err)
		}
		<-shutdownChan
	}
	return n
}
