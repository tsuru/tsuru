// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	stdLog "log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tsuru/config"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image/gc"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	_ "github.com/tsuru/tsuru/auth/saml"
	"github.com/tsuru/tsuru/autoscale"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/webhook"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	"golang.org/x/net/websocket"
)

const Version = "1.5.0-rc11"

type TsuruHandler struct {
	version string
	method  string
	path    string
	h       http.Handler
}

func fatal(err error) {
	fmt.Println(err.Error())
	log.Fatal(err.Error())
}

var tsuruHandlerList []TsuruHandler

// RegisterHandler inserts a handler on a list of handlers for version 1.0
func RegisterHandler(path string, method string, h http.Handler) {
	RegisterHandlerVersion("1.0", path, method, h)
}

// RegisterHandlerVersion inserts a handler on a list of handlers
func RegisterHandlerVersion(version, path, method string, h http.Handler) {
	var th TsuruHandler
	th.version = version
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

func setupServices() error {
	var err error
	servicemanager.TeamToken, err = auth.TeamTokenService()
	if err != nil {
		return err
	}
	servicemanager.Cache, err = app.CacheService()
	if err != nil {
		return err
	}
	servicemanager.Team, err = auth.TeamService()
	if err != nil {
		return err
	}
	servicemanager.Plan, err = app.PlanService()
	if err != nil {
		return err
	}
	servicemanager.Platform, err = app.PlatformService()
	if err != nil {
		return err
	}
	servicemanager.WebHook, err = webhook.WebHookService()
	return err
}

// RunServer starts tsuru API server. The dry parameter indicates whether the
// server should run in dry mode, not starting the HTTP listener (for testing
// purposes).
func RunServer(dry bool) http.Handler {
	err := log.Init()
	if err != nil {
		stdLog.Fatalf("unable to initialize logging: %v", err)
	}
	err = setupDatabase()
	if err != nil {
		fatal(err)
	}
	err = setupServices()
	if err != nil {
		fatal(err)
	}

	m := apiRouter.NewRouter()

	for _, handler := range tsuruHandlerList {
		m.Add(handler.version, handler.method, handler.path, handler.h)
	}

	if disableIndex, _ := config.GetBool("disable-index-page"); !disableIndex {
		m.Add("1.0", "Get", "/", Handler(index))
	}
	m.Add("1.0", "Get", "/info", Handler(info))

	m.Add("1.0", "Get", "/services/instances", AuthorizationRequiredHandler(serviceInstances))
	m.Add("1.0", "Get", "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(serviceInstance))
	m.Add("1.0", "Delete", "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(removeServiceInstance))
	m.Add("1.0", "Post", "/services/{service}/instances", AuthorizationRequiredHandler(createServiceInstance))
	m.Add("1.0", "Put", "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(updateServiceInstance))
	m.Add("1.0", "Put", "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(bindServiceInstance))
	m.Add("1.0", "Delete", "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(unbindServiceInstance))
	m.Add("1.0", "Get", "/services/{service}/instances/{instance}/status", AuthorizationRequiredHandler(serviceInstanceStatus))
	m.Add("1.0", "Put", "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceGrantTeam))
	m.Add("1.0", "Delete", "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceRevokeTeam))

	m.AddAll("1.0", "/services/{service}/proxy/{instance}", AuthorizationRequiredHandler(serviceInstanceProxy))
	m.AddAll("1.0", "/services/proxy/service/{service}", AuthorizationRequiredHandler(serviceProxy))

	m.Add("1.0", "Get", "/services", AuthorizationRequiredHandler(serviceList))
	m.Add("1.0", "Post", "/services", AuthorizationRequiredHandler(serviceCreate))
	m.Add("1.0", "Put", "/services/{name}", AuthorizationRequiredHandler(serviceUpdate))
	m.Add("1.0", "Delete", "/services/{name}", AuthorizationRequiredHandler(serviceDelete))
	m.Add("1.0", "Get", "/services/{name}", AuthorizationRequiredHandler(serviceInfo))
	m.Add("1.0", "Get", "/services/{name}/plans", AuthorizationRequiredHandler(servicePlans))
	m.Add("1.0", "Get", "/services/{name}/doc", AuthorizationRequiredHandler(serviceDoc))
	m.Add("1.0", "Put", "/services/{name}/doc", AuthorizationRequiredHandler(serviceAddDoc))
	m.Add("1.0", "Put", "/services/{service}/team/{team}", AuthorizationRequiredHandler(grantServiceAccess))
	m.Add("1.0", "Delete", "/services/{service}/team/{team}", AuthorizationRequiredHandler(revokeServiceAccess))

	m.Add("1.0", "Delete", "/apps/{app}", AuthorizationRequiredHandler(appDelete))
	m.Add("1.0", "Get", "/apps/{app}", AuthorizationRequiredHandler(appInfo))
	m.Add("1.0", "Post", "/apps/{app}/cname", AuthorizationRequiredHandler(setCName))
	m.Add("1.0", "Delete", "/apps/{app}/cname", AuthorizationRequiredHandler(unsetCName))
	runHandler := AuthorizationRequiredHandler(runCommand)
	m.Add("1.0", "Post", "/apps/{app}/run", runHandler)
	m.Add("1.0", "Post", "/apps/{app}/restart", AuthorizationRequiredHandler(restart))
	m.Add("1.0", "Post", "/apps/{app}/start", AuthorizationRequiredHandler(start))
	m.Add("1.0", "Post", "/apps/{app}/stop", AuthorizationRequiredHandler(stop))
	m.Add("1.0", "Post", "/apps/{app}/sleep", AuthorizationRequiredHandler(sleep))
	m.Add("1.0", "Get", "/apps/{appname}/quota", AuthorizationRequiredHandler(getAppQuota))
	m.Add("1.0", "Put", "/apps/{appname}/quota", AuthorizationRequiredHandler(changeAppQuota))
	m.Add("1.0", "Put", "/apps/{appname}", AuthorizationRequiredHandler(updateApp))
	m.Add("1.0", "Get", "/apps/{app}/env", AuthorizationRequiredHandler(getEnv))
	m.Add("1.0", "Post", "/apps/{app}/env", AuthorizationRequiredHandler(setEnv))
	m.Add("1.0", "Delete", "/apps/{app}/env", AuthorizationRequiredHandler(unsetEnv))
	m.Add("1.0", "Get", "/apps", AuthorizationRequiredHandler(appList))
	m.Add("1.0", "Post", "/apps", AuthorizationRequiredHandler(createApp))
	forceDeleteLockHandler := AuthorizationRequiredHandler(forceDeleteLock)
	m.Add("1.0", "Delete", "/apps/{app}/lock", forceDeleteLockHandler)
	m.Add("1.0", "Put", "/apps/{app}/units", AuthorizationRequiredHandler(addUnits))
	m.Add("1.0", "Delete", "/apps/{app}/units", AuthorizationRequiredHandler(removeUnits))
	registerUnitHandler := AuthorizationRequiredHandler(registerUnit)
	m.Add("1.0", "Post", "/apps/{app}/units/register", registerUnitHandler)
	setUnitStatusHandler := AuthorizationRequiredHandler(setUnitStatus)
	m.Add("1.0", "Post", "/apps/{app}/units/{unit}", setUnitStatusHandler)
	m.Add("1.0", "Put", "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(grantAppAccess))
	m.Add("1.0", "Delete", "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(revokeAppAccess))
	m.Add("1.0", "Get", "/apps/{app}/log", AuthorizationRequiredHandler(appLog))
	logPostHandler := AuthorizationRequiredHandler(addLog)
	m.Add("1.0", "Post", "/apps/{app}/log", logPostHandler)
	m.Add("1.0", "Post", "/apps/{appname}/deploy/rollback", AuthorizationRequiredHandler(deployRollback))
	m.Add("1.4", "Put", "/apps/{appname}/deploy/rollback/update", AuthorizationRequiredHandler(deployRollbackUpdate))
	m.Add("1.3", "Post", "/apps/{appname}/deploy/rebuild", AuthorizationRequiredHandler(deployRebuild))
	m.Add("1.0", "Get", "/apps/{app}/metric/envs", AuthorizationRequiredHandler(appMetricEnvs))
	m.Add("1.0", "Post", "/apps/{app}/routes", AuthorizationRequiredHandler(appRebuildRoutes))
	m.Add("1.2", "Get", "/apps/{app}/certificate", AuthorizationRequiredHandler(listCertificates))
	m.Add("1.2", "Put", "/apps/{app}/certificate", AuthorizationRequiredHandler(setCertificate))
	m.Add("1.2", "Delete", "/apps/{app}/certificate", AuthorizationRequiredHandler(unsetCertificate))

	m.Add("1.5", "Post", "/apps/{app}/routers", AuthorizationRequiredHandler(addAppRouter))
	m.Add("1.5", "Put", "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(updateAppRouter))
	m.Add("1.5", "Delete", "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(removeAppRouter))
	m.Add("1.5", "Get", "/apps/{app}/routers", AuthorizationRequiredHandler(listAppRouters))

	m.Add("1.0", "Post", "/node/status", AuthorizationRequiredHandler(setNodeStatus))

	m.Add("1.0", "Get", "/deploys", AuthorizationRequiredHandler(deploysList))
	m.Add("1.0", "Get", "/deploys/{deploy}", AuthorizationRequiredHandler(deployInfo))

	m.Add("1.1", "Get", "/events", AuthorizationRequiredHandler(eventList))
	m.Add("1.3", "Get", "/events/blocks", AuthorizationRequiredHandler(eventBlockList))
	m.Add("1.3", "Post", "/events/blocks", AuthorizationRequiredHandler(eventBlockAdd))
	m.Add("1.3", "Delete", "/events/blocks/{uuid}", AuthorizationRequiredHandler(eventBlockRemove))
	m.Add("1.1", "Get", "/events/kinds", AuthorizationRequiredHandler(kindList))
	m.Add("1.1", "Get", "/events/{uuid}", AuthorizationRequiredHandler(eventInfo))
	m.Add("1.1", "Post", "/events/{uuid}/cancel", AuthorizationRequiredHandler(eventCancel))

	m.Add("1.6", "Get", "/events/webhooks", AuthorizationRequiredHandler(webhookList))
	m.Add("1.6", "Post", "/events/webhooks", AuthorizationRequiredHandler(webhookCreate))
	m.Add("1.6", "Get", "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookInfo))
	m.Add("1.6", "Put", "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookUpdate))
	m.Add("1.6", "Delete", "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookDelete))

	m.Add("1.0", "Get", "/platforms", AuthorizationRequiredHandler(platformList))
	m.Add("1.0", "Post", "/platforms", AuthorizationRequiredHandler(platformAdd))
	m.Add("1.0", "Put", "/platforms/{name}", AuthorizationRequiredHandler(platformUpdate))
	m.Add("1.0", "Delete", "/platforms/{name}", AuthorizationRequiredHandler(platformRemove))

	// These handlers don't use {app} on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Add("1.0", "Post", "/apps/{appname}/repository/clone", AuthorizationRequiredHandler(deploy))
	m.Add("1.0", "Post", "/apps/{appname}/deploy", AuthorizationRequiredHandler(deploy))
	diffDeployHandler := AuthorizationRequiredHandler(diffDeploy)
	m.Add("1.0", "Post", "/apps/{appname}/diff", diffDeployHandler)
	m.Add("1.5", "Post", "/apps/{appname}/build", AuthorizationRequiredHandler(build))

	// Shell also doesn't use {app} on purpose. Middlewares don't play well
	// with websocket.
	m.Add("1.0", "Get", "/apps/{appname}/shell", http.HandlerFunc(remoteShellHandler))

	m.Add("1.0", "Get", "/users", AuthorizationRequiredHandler(listUsers))
	m.Add("1.0", "Post", "/users", Handler(createUser))
	m.Add("1.0", "Get", "/users/info", AuthorizationRequiredHandler(userInfo))
	m.Add("1.0", "Get", "/auth/scheme", Handler(authScheme))
	m.Add("1.0", "Post", "/auth/login", Handler(login))

	m.Add("1.0", "Post", "/auth/saml", Handler(samlCallbackLogin))
	m.Add("1.0", "Get", "/auth/saml", Handler(samlMetadata))

	m.Add("1.0", "Post", "/users/{email}/password", Handler(resetPassword))
	m.Add("1.0", "Post", "/users/{email}/tokens", Handler(login))
	m.Add("1.0", "Get", "/users/{email}/quota", AuthorizationRequiredHandler(getUserQuota))
	m.Add("1.0", "Put", "/users/{email}/quota", AuthorizationRequiredHandler(changeUserQuota))
	m.Add("1.0", "Delete", "/users/tokens", AuthorizationRequiredHandler(logout))
	m.Add("1.0", "Put", "/users/password", AuthorizationRequiredHandler(changePassword))
	m.Add("1.0", "Delete", "/users", AuthorizationRequiredHandler(removeUser))
	m.Add("1.0", "Get", "/users/keys", AuthorizationRequiredHandler(listKeys))
	m.Add("1.0", "Post", "/users/keys", AuthorizationRequiredHandler(addKeyToUser))
	m.Add("1.0", "Delete", "/users/keys/{key}", AuthorizationRequiredHandler(removeKeyFromUser))
	m.Add("1.0", "Get", "/users/api-key", AuthorizationRequiredHandler(showAPIToken))
	m.Add("1.0", "Post", "/users/api-key", AuthorizationRequiredHandler(regenerateAPIToken))

	m.Add("1.0", "Get", "/logs", websocket.Handler(addLogs))

	m.Add("1.0", "Get", "/teams", AuthorizationRequiredHandler(teamList))
	m.Add("1.0", "Post", "/teams", AuthorizationRequiredHandler(createTeam))
	m.Add("1.0", "Delete", "/teams/{name}", AuthorizationRequiredHandler(removeTeam))
	m.Add("1.6", "Put", "/teams/{name}", AuthorizationRequiredHandler(updateTeam))
	m.Add("1.4", "Get", "/teams/{name}", AuthorizationRequiredHandler(teamInfo))

	m.Add("1.0", "Post", "/swap", AuthorizationRequiredHandler(swap))

	m.Add("1.0", "Get", "/healthcheck/", http.HandlerFunc(healthcheck))
	m.Add("1.0", "Get", "/healthcheck", http.HandlerFunc(healthcheck))

	m.Add("1.0", "Get", "/iaas/machines", AuthorizationRequiredHandler(machinesList))
	m.Add("1.0", "Delete", "/iaas/machines/{machine_id}", AuthorizationRequiredHandler(machineDestroy))
	m.Add("1.0", "Get", "/iaas/templates", AuthorizationRequiredHandler(templatesList))
	m.Add("1.0", "Post", "/iaas/templates", AuthorizationRequiredHandler(templateCreate))
	m.Add("1.0", "Put", "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateUpdate))
	m.Add("1.0", "Delete", "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateDestroy))

	m.Add("1.0", "Get", "/plans", AuthorizationRequiredHandler(listPlans))
	m.Add("1.0", "Post", "/plans", AuthorizationRequiredHandler(addPlan))
	m.Add("1.0", "Delete", "/plans/{planname}", AuthorizationRequiredHandler(removePlan))

	m.Add("1.0", "Get", "/pools", AuthorizationRequiredHandler(poolList))
	m.Add("1.0", "Post", "/pools", AuthorizationRequiredHandler(addPoolHandler))
	m.Add("1.0", "Delete", "/pools/{name}", AuthorizationRequiredHandler(removePoolHandler))
	m.Add("1.0", "Put", "/pools/{name}", AuthorizationRequiredHandler(poolUpdateHandler))
	m.Add("1.0", "Post", "/pools/{name}/team", AuthorizationRequiredHandler(addTeamToPoolHandler))
	m.Add("1.0", "Delete", "/pools/{name}/team", AuthorizationRequiredHandler(removeTeamToPoolHandler))

	m.Add("1.3", "Get", "/constraints", AuthorizationRequiredHandler(poolConstraintList))
	m.Add("1.3", "Put", "/constraints", AuthorizationRequiredHandler(poolConstraintSet))

	m.Add("1.0", "Get", "/roles", AuthorizationRequiredHandler(listRoles))
	m.Add("1.4", "Put", "/roles", AuthorizationRequiredHandler(roleUpdate))
	m.Add("1.0", "Post", "/roles", AuthorizationRequiredHandler(addRole))
	m.Add("1.0", "Get", "/roles/{name}", AuthorizationRequiredHandler(roleInfo))
	m.Add("1.0", "Delete", "/roles/{name}", AuthorizationRequiredHandler(removeRole))
	m.Add("1.0", "Post", "/roles/{name}/permissions", AuthorizationRequiredHandler(addPermissions))
	m.Add("1.0", "Delete", "/roles/{name}/permissions/{permission}", AuthorizationRequiredHandler(removePermissions))
	m.Add("1.0", "Post", "/roles/{name}/user", AuthorizationRequiredHandler(assignRole))
	m.Add("1.0", "Delete", "/roles/{name}/user/{email}", AuthorizationRequiredHandler(dissociateRole))
	m.Add("1.0", "Get", "/role/default", AuthorizationRequiredHandler(listDefaultRoles))
	m.Add("1.0", "Post", "/role/default", AuthorizationRequiredHandler(addDefaultRole))
	m.Add("1.0", "Delete", "/role/default", AuthorizationRequiredHandler(removeDefaultRole))
	m.Add("1.0", "Get", "/permissions", AuthorizationRequiredHandler(listPermissions))
	m.Add("1.6", "Post", "/roles/{name}/token", AuthorizationRequiredHandler(assignRoleToToken))
	m.Add("1.6", "Delete", "/roles/{name}/token/{token_id}", AuthorizationRequiredHandler(dissociateRoleFromToken))

	m.Add("1.0", "Get", "/debug/goroutines", AuthorizationRequiredHandler(dumpGoroutines))
	m.Add("1.0", "Get", "/debug/pprof/", AuthorizationRequiredHandler(indexHandler))
	m.Add("1.0", "Get", "/debug/pprof/cmdline", AuthorizationRequiredHandler(cmdlineHandler))
	m.Add("1.0", "Get", "/debug/pprof/profile", AuthorizationRequiredHandler(profileHandler))
	m.Add("1.0", "Get", "/debug/pprof/symbol", AuthorizationRequiredHandler(symbolHandler))
	m.Add("1.0", "Get", "/debug/pprof/heap", AuthorizationRequiredHandler(indexHandler))
	m.Add("1.0", "Get", "/debug/pprof/goroutine", AuthorizationRequiredHandler(indexHandler))
	m.Add("1.0", "Get", "/debug/pprof/threadcreate", AuthorizationRequiredHandler(indexHandler))
	m.Add("1.0", "Get", "/debug/pprof/block", AuthorizationRequiredHandler(indexHandler))
	m.Add("1.0", "Get", "/debug/pprof/trace", AuthorizationRequiredHandler(traceHandler))

	m.Add("1.3", "GET", "/node/autoscale", AuthorizationRequiredHandler(autoScaleHistoryHandler))
	m.Add("1.3", "GET", "/node/autoscale/config", AuthorizationRequiredHandler(autoScaleGetConfig))
	m.Add("1.3", "POST", "/node/autoscale/run", AuthorizationRequiredHandler(autoScaleRunHandler))
	m.Add("1.3", "GET", "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleListRules))
	m.Add("1.3", "POST", "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleSetRule))
	m.Add("1.3", "DELETE", "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleDeleteRule))
	m.Add("1.3", "DELETE", "/node/autoscale/rules/{id}", AuthorizationRequiredHandler(autoScaleDeleteRule))

	m.Add("1.2", "GET", "/node", AuthorizationRequiredHandler(listNodesHandler))
	m.Add("1.2", "GET", "/node/apps/{appname}/containers", AuthorizationRequiredHandler(listUnitsByApp))
	m.Add("1.2", "GET", "/node/{address:.*}/containers", AuthorizationRequiredHandler(listUnitsByNode))
	m.Add("1.2", "POST", "/node", AuthorizationRequiredHandler(addNodeHandler))
	m.Add("1.2", "PUT", "/node", AuthorizationRequiredHandler(updateNodeHandler))
	m.Add("1.2", "DELETE", "/node/{address:.*}", AuthorizationRequiredHandler(removeNodeHandler))
	m.Add("1.3", "POST", "/node/rebalance", AuthorizationRequiredHandler(rebalanceNodesHandler))
	m.Add("1.6", "GET", "/node/{address:.*}", AuthorizationRequiredHandler(infoNodeHandler))

	m.Add("1.2", "GET", "/nodecontainers", AuthorizationRequiredHandler(nodeContainerList))
	m.Add("1.2", "POST", "/nodecontainers", AuthorizationRequiredHandler(nodeContainerCreate))
	m.Add("1.2", "GET", "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerInfo))
	m.Add("1.2", "DELETE", "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerDelete))
	m.Add("1.2", "POST", "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerUpdate))
	m.Add("1.2", "POST", "/nodecontainers/{name}/upgrade", AuthorizationRequiredHandler(nodeContainerUpgrade))

	m.Add("1.2", "POST", "/install/hosts", AuthorizationRequiredHandler(installHostAdd))
	m.Add("1.2", "GET", "/install/hosts", AuthorizationRequiredHandler(installHostList))
	m.Add("1.2", "GET", "/install/hosts/{name}", AuthorizationRequiredHandler(installHostInfo))

	m.Add("1.2", "GET", "/healing/node", AuthorizationRequiredHandler(nodeHealingRead))
	m.Add("1.2", "POST", "/healing/node", AuthorizationRequiredHandler(nodeHealingUpdate))
	m.Add("1.2", "DELETE", "/healing/node", AuthorizationRequiredHandler(nodeHealingDelete))
	m.Add("1.3", "GET", "/healing", AuthorizationRequiredHandler(healingHistoryHandler))
	m.Add("1.3", "GET", "/routers", AuthorizationRequiredHandler(listRouters))
	m.Add("1.2", "GET", "/metrics", promhttp.Handler())

	m.Add("1.3", "POST", "/provisioner/clusters", AuthorizationRequiredHandler(createCluster))
	m.Add("1.4", "POST", "/provisioner/clusters/{name}", AuthorizationRequiredHandler(updateCluster))
	m.Add("1.3", "GET", "/provisioner/clusters", AuthorizationRequiredHandler(listClusters))
	m.Add("1.3", "DELETE", "/provisioner/clusters/{name}", AuthorizationRequiredHandler(deleteCluster))

	m.Add("1.4", "GET", "/volumes", AuthorizationRequiredHandler(volumesList))
	m.Add("1.4", "GET", "/volumes/{name}", AuthorizationRequiredHandler(volumeInfo))
	m.Add("1.4", "DELETE", "/volumes/{name}", AuthorizationRequiredHandler(volumeDelete))
	m.Add("1.4", "POST", "/volumes", AuthorizationRequiredHandler(volumeCreate))
	m.Add("1.4", "POST", "/volumes/{name}", AuthorizationRequiredHandler(volumeUpdate))
	m.Add("1.4", "POST", "/volumes/{name}/bind", AuthorizationRequiredHandler(volumeBind))
	m.Add("1.4", "DELETE", "/volumes/{name}/bind", AuthorizationRequiredHandler(volumeUnbind))
	m.Add("1.4", "GET", "/volumeplans", AuthorizationRequiredHandler(volumePlansList))

	m.Add("1.6", "GET", "/tokens", AuthorizationRequiredHandler(tokenList))
	m.Add("1.6", "POST", "/tokens", AuthorizationRequiredHandler(tokenCreate))
	m.Add("1.6", "DELETE", "/tokens/{token_id}", AuthorizationRequiredHandler(tokenDelete))
	m.Add("1.6", "PUT", "/tokens/{token_id}", AuthorizationRequiredHandler(tokenUpdate))

	// Handlers for compatibility reasons, should be removed on tsuru 2.0.
	m.Add("1.4", "Post", "/teams/{name}", AuthorizationRequiredHandler(updateTeam))
	m.Add("1.0", "GET", "/docker/node", AuthorizationRequiredHandler(listNodesHandler))
	m.Add("1.0", "GET", "/docker/node/apps/{appname}/containers", AuthorizationRequiredHandler(listUnitsByApp))
	m.Add("1.0", "GET", "/docker/node/{address:.*}/containers", AuthorizationRequiredHandler(listUnitsByNode))
	m.Add("1.0", "POST", "/docker/node", AuthorizationRequiredHandler(addNodeHandler))
	m.Add("1.0", "PUT", "/docker/node", AuthorizationRequiredHandler(updateNodeHandler))
	m.Add("1.0", "DELETE", "/docker/node/{address:.*}", AuthorizationRequiredHandler(removeNodeHandler))
	m.Add("1.0", "POST", "/docker/containers/rebalance", AuthorizationRequiredHandler(rebalanceNodesHandler))

	m.Add("1.0", "GET", "/docker/nodecontainers", AuthorizationRequiredHandler(nodeContainerList))
	m.Add("1.0", "POST", "/docker/nodecontainers", AuthorizationRequiredHandler(nodeContainerCreate))
	m.Add("1.0", "GET", "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerInfo))
	m.Add("1.0", "DELETE", "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerDelete))
	m.Add("1.0", "POST", "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerUpdate))
	m.Add("1.0", "POST", "/docker/nodecontainers/{name}/upgrade", AuthorizationRequiredHandler(nodeContainerUpgrade))

	m.Add("1.0", "GET", "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingRead))
	m.Add("1.0", "POST", "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingUpdate))
	m.Add("1.0", "DELETE", "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingDelete))
	m.Add("1.0", "GET", "/docker/healing", AuthorizationRequiredHandler(healingHistoryHandler))

	m.Add("1.0", "GET", "/docker/autoscale", AuthorizationRequiredHandler(autoScaleHistoryHandler))
	m.Add("1.0", "GET", "/docker/autoscale/config", AuthorizationRequiredHandler(autoScaleGetConfig))
	m.Add("1.0", "POST", "/docker/autoscale/run", AuthorizationRequiredHandler(autoScaleRunHandler))
	m.Add("1.0", "GET", "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleListRules))
	m.Add("1.0", "POST", "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleSetRule))
	m.Add("1.0", "DELETE", "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleDeleteRule))
	m.Add("1.0", "DELETE", "/docker/autoscale/rules/{id}", AuthorizationRequiredHandler(autoScaleDeleteRule))

	m.Add("1.0", "GET", "/plans/routers", AuthorizationRequiredHandler(listRouters))

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(negroni.HandlerFunc(contextClearerMiddleware))
	if !dry {
		n.Use(newLoggerMiddleware())
	}
	n.UseHandler(m)
	n.Use(negroni.HandlerFunc(flushingWriterMiddleware))
	n.Use(negroni.HandlerFunc(setRequestIDHeaderMiddleware))
	n.Use(negroni.HandlerFunc(errorHandlingMiddleware))
	n.Use(negroni.HandlerFunc(setVersionHeadersMiddleware))
	n.Use(negroni.HandlerFunc(authTokenMiddleware))
	n.Use(negroni.HandlerFunc(contentHijacker))
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
		err := startServer(n)
		if err != nil {
			fatal(err)
		}
	}
	return n
}

func setupDatabase() error {
	connString, err := config.GetString("database:url")
	if err != nil {
		connString = db.DefaultDatabaseURL
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		dbName = db.DefaultDatabaseName
	}
	dbDriverName, err := config.GetString("database:driver")
	if err != nil {
		dbDriverName = storage.DefaultDbDriverName
		fmt.Println("Warning: configuration didn't declare a database driver, using default driver.")
	}
	fmt.Printf("Using %q database %q from the server %q.\n", dbDriverName, dbName, connString)
	_, err = storage.GetDbDriver(dbDriverName)
	if err != nil {
		return err
	}
	if dbDriverName != storage.DefaultDbDriverName {
		_, err = storage.GetDefaultDbDriver()
		if err != nil {
			return err
		}
	}
	return nil
}

func appFinder(appName string) (rebuild.RebuildApp, error) {
	a, err := app.GetByName(appName)
	if err == app.ErrAppNotFound {
		return nil, nil
	}
	return a, err
}

func bindAppsLister() ([]bind.App, error) {
	apps, err := app.List(nil)
	if err != nil {
		return nil, err
	}
	bindApps := make([]bind.App, len(apps))
	for i := range apps {
		bindApps[i] = &apps[i]
	}
	return bindApps, nil
}

func startServer(handler http.Handler) error {
	srvConf, err := createServers(handler)
	if err != nil {
		return err
	}
	shutdownTimeoutInt, _ := config.GetInt("shutdown-timeout")
	shutdownTimeout := 10 * time.Minute
	if shutdownTimeoutInt != 0 {
		shutdownTimeout = time.Duration(shutdownTimeoutInt) * time.Second
	}
	go srvConf.handleSignals(shutdownTimeout)

	defer func() {
		srvConf.shutdown(shutdownTimeout)
		fmt.Println("tsuru is running shutdown handlers")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		shutdown.Do(ctx, os.Stdout)
		cancel()
	}()

	shutdown.Register(&logTracker)
	var startupMessage string
	err = router.Initialize()
	if err != nil {
		return err
	}
	routers, err := router.List()
	if err != nil {
		return err
	}
	for _, routerDesc := range routers {
		var r router.Router
		r, err = router.Get(routerDesc.Name)
		if err != nil {
			return err
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
	defaultRouter, _ := router.Default()
	fmt.Printf("Default router is %q.\n", defaultRouter)
	repoManager, err := config.GetString("repo-manager")
	if err != nil {
		repoManager = "gandalf"
		fmt.Println("Warning: configuration didn't declare a repository manager, using default manager.")
	}
	fmt.Printf("Using %q repository manager.\n", repoManager)
	err = rebuild.RegisterTask(appFinder)
	if err != nil {
		return err
	}
	scheme, err := getAuthScheme()
	if err != nil {
		fmt.Printf("Warning: configuration didn't declare auth:scheme, using default scheme.\n")
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	fmt.Printf("Using %q auth scheme.\n", scheme)
	_, err = nodecontainer.InitializeBS(app.AuthScheme, app.InternalAppName)
	if err != nil {
		return err
	}
	err = provision.InitializeAll()
	if err != nil {
		return err
	}
	_, err = healer.Initialize()
	if err != nil {
		return err
	}
	err = autoscale.Initialize()
	if err != nil {
		return err
	}
	err = event.Initialize()
	if err != nil {
		return errors.Wrap(err, "unable to load events throttling config")
	}
	err = gc.Initialize()
	if err != nil {
		return errors.Wrap(err, "unable to initialize old image gc")
	}
	err = service.InitializeSync(bindAppsLister)
	if err != nil {
		return err
	}
	fmt.Println("Checking components status:")
	results := hc.Check("all")
	for _, result := range results {
		if result.Status != hc.HealthCheckOK {
			fmt.Printf("    WARNING: %q is not working: %s\n", result.Name, result.Status)
		}
	}
	fmt.Println("    Components checked.")

	err = <-srvConf.start()
	if err != http.ErrServerClosed {
		return errors.Wrap(err, "unexpected error in server while listening")
	}
	fmt.Printf("Listening stopped: %s\n", err)
	return nil
}

func createServers(handler http.Handler) (*srvConfig, error) {
	var srvConf srvConfig
	var err error
	useTls, _ := config.GetBool("use-tls")
	tlsListen, _ := config.GetString("tls:listen")
	listen, _ := config.GetString("listen")
	if useTls {
		if tlsListen == "" && listen != "" {
			tlsListen = listen
			listen = ""
		}
		srvConf.certFile, err = config.GetString("tls:cert-file")
		if err != nil {
			return nil, err
		}
		srvConf.keyFile, err = config.GetString("tls:key-file")
		if err != nil {
			return nil, err
		}
	} else if listen == "" {
		return nil, errors.New(`missing "listen" config key`)
	}
	readTimeout, _ := config.GetInt("server:read-timeout")
	writeTimeout, _ := config.GetInt("server:write-timeout")
	if listen != "" {
		srvConf.httpSrv = &http.Server{
			ReadTimeout:  time.Duration(readTimeout) * time.Second,
			WriteTimeout: time.Duration(writeTimeout) * time.Second,
			Addr:         listen,
			Handler:      handler,
		}
	}
	if tlsListen != "" {
		srvConf.httpsSrv = &http.Server{
			ReadTimeout:  time.Duration(readTimeout) * time.Second,
			WriteTimeout: time.Duration(writeTimeout) * time.Second,
			Addr:         tlsListen,
			Handler:      handler,
		}
	}
	return &srvConf, nil
}

type srvConfig struct {
	httpSrv  *http.Server
	httpsSrv *http.Server
	certFile string
	keyFile  string
}

func (conf *srvConfig) shutdown(shutdownTimeout time.Duration) {
	wg := sync.WaitGroup{}
	shutdownSrv := func(srv *http.Server) {
		defer wg.Done()
		fmt.Printf("tsuru is shutting down server %v, waiting for pending connections to finish.\n", srv.Addr)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		srv.Shutdown(ctx)
	}
	if conf.httpSrv != nil {
		wg.Add(1)
		go shutdownSrv(conf.httpSrv)
	}
	if conf.httpsSrv != nil {
		wg.Add(1)
		go shutdownSrv(conf.httpsSrv)
	}
	wg.Wait()
}

func (conf *srvConfig) handleSignals(shutdownTimeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	conf.shutdown(shutdownTimeout)
}

func (conf *srvConfig) start() <-chan error {
	errChan := make(chan error, 2)
	if conf.httpSrv != nil {
		go func() {
			fmt.Printf("tsuru HTTP server listening at %s...\n", conf.httpSrv.Addr)
			errChan <- conf.httpSrv.ListenAndServe()
		}()
	}
	if conf.httpsSrv != nil {
		go func() {
			fmt.Printf("tsuru HTTP/TLS server listening at %s...\n", conf.httpsSrv.Addr)
			errChan <- conf.httpsSrv.ListenAndServeTLS(conf.certFile, conf.keyFile)
		}()
	}
	return errChan
}
