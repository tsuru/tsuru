// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	stdLog "log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ajg/form"
	"github.com/codegangsta/negroni"
	"github.com/felixge/fgprof"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/observability"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/api/tracker"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/image/gc"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/applog"
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
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/volume"
	"golang.org/x/net/websocket"
)

const Version = "1.11.5"

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

var onceServices sync.Once

func setupServices() error {
	var err error
	servicemanager.App, err = app.AppService()
	if err != nil {
		return err
	}
	servicemanager.TeamToken, err = auth.TeamTokenService()
	if err != nil {
		return err
	}
	servicemanager.AppCache, err = app.CacheService()
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
	servicemanager.PlatformImage, err = image.PlatformImageService()
	if err != nil {
		return err
	}
	servicemanager.UserQuota, err = auth.UserQuotaService()
	if err != nil {
		return err
	}
	servicemanager.AppQuota, err = app.QuotaService()
	if err != nil {
		return err
	}
	servicemanager.TeamQuota, err = auth.TeamQuotaService()
	if err != nil {
		return err
	}
	servicemanager.Webhook, err = webhook.WebhookService()
	if err != nil {
		return err
	}
	servicemanager.Cluster, err = cluster.ClusterService()
	if err != nil {
		return err
	}
	servicemanager.ServiceBroker, err = service.BrokerService()
	if err != nil {
		return err
	}
	servicemanager.ServiceBrokerCatalogCache, err = service.CatalogCacheService()
	if err != nil {
		return err
	}
	servicemanager.AppLog, err = applog.AppLogService()
	if err != nil {
		return err
	}
	servicemanager.InstanceTracker, err = tracker.InstanceService()
	if err != nil {
		return err
	}
	servicemanager.DynamicRouter, err = router.DynamicRouterService()
	if err != nil {
		return err
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	if err != nil {
		return err
	}
	servicemanager.AuthGroup, err = auth.GroupService()
	if err != nil {
		return err
	}
	servicemanager.Pool, err = pool.PoolService()
	if err != nil {
		return err
	}
	servicemanager.Volume, err = volume.VolumeService()
	if err != nil {
		return err
	}
	return nil
}

func InitializeDBServices() error {
	err := setupDatabase()
	if err != nil {
		return err
	}
	onceServices.Do(func() {
		err = setupServices()
	})
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
	err = InitializeDBServices()
	if err != nil {
		fatal(err)
	}
	m := apiRouter.NewRouter()

	for _, handler := range tsuruHandlerList {
		m.Add(handler.version, handler.method, handler.path, handler.h)
	}

	if disableIndex, _ := config.GetBool("disable-index-page"); !disableIndex {
		m.Add("1.0", http.MethodGet, "/", Handler(index))
	}
	m.Add("1.0", http.MethodGet, "/info", AuthorizationRequiredHandler(info))

	m.Add("1.0", http.MethodGet, "/services/instances", AuthorizationRequiredHandler(serviceInstances))
	m.Add("1.0", http.MethodPost, "/services/{service}/instances", AuthorizationRequiredHandler(createServiceInstance))
	m.Add("1.0", http.MethodGet, "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(serviceInstance))
	m.Add("1.0", http.MethodPut, "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(updateServiceInstance))
	m.Add("1.0", http.MethodDelete, "/services/{service}/instances/{instance}", AuthorizationRequiredHandler(removeServiceInstance))
	m.Add("1.0", http.MethodGet, "/services/{service}/instances/{instance}/status", AuthorizationRequiredHandler(serviceInstanceStatus))
	m.Add("1.0", http.MethodPut, "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(bindServiceInstance))
	m.Add("1.0", http.MethodDelete, "/services/{service}/instances/{instance}/{app}", AuthorizationRequiredHandler(unbindServiceInstance))
	m.Add("1.0", http.MethodPut, "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceGrantTeam))
	m.Add("1.0", http.MethodDelete, "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceRevokeTeam))

	m.AddAll("1.0", "/services/{service}/proxy/{instance}", AuthorizationRequiredHandler(serviceInstanceProxy))
	m.AddAll("1.0", "/services/proxy/service/{service}", AuthorizationRequiredHandler(serviceProxy))

	m.Add("1.0", http.MethodGet, "/services", AuthorizationRequiredHandler(serviceList))
	m.Add("1.0", http.MethodPost, "/services", AuthorizationRequiredHandler(serviceCreate))
	m.Add("1.0", http.MethodPut, "/services/{name}", AuthorizationRequiredHandler(serviceUpdate))
	m.Add("1.0", http.MethodDelete, "/services/{name}", AuthorizationRequiredHandler(serviceDelete))
	m.Add("1.0", http.MethodGet, "/services/{name}", AuthorizationRequiredHandler(serviceInfo))
	m.Add("1.0", http.MethodGet, "/services/{name}/plans", AuthorizationRequiredHandler(servicePlans))
	m.Add("1.0", http.MethodGet, "/services/{name}/doc", AuthorizationRequiredHandler(serviceDoc))
	m.Add("1.0", http.MethodPut, "/services/{name}/doc", AuthorizationRequiredHandler(serviceAddDoc))
	m.Add("1.0", http.MethodPut, "/services/{service}/team/{team}", AuthorizationRequiredHandler(grantServiceAccess))
	m.Add("1.0", http.MethodDelete, "/services/{service}/team/{team}", AuthorizationRequiredHandler(revokeServiceAccess))

	m.Add("1.0", http.MethodGet, "/apps", AuthorizationRequiredHandler(appList))
	m.Add("1.0", http.MethodPost, "/apps", AuthorizationRequiredHandler(createApp))
	m.Add("1.0", http.MethodGet, "/apps/{app}", AuthorizationRequiredHandler(appInfo))
	m.Add("1.0", http.MethodDelete, "/apps/{app}", AuthorizationRequiredHandler(appDelete))
	m.Add("1.0", http.MethodPut, "/apps/{app}", AuthorizationRequiredHandler(updateApp))
	m.Add("1.0", http.MethodPost, "/apps/{app}/cname", AuthorizationRequiredHandler(setCName))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/cname", AuthorizationRequiredHandler(unsetCName))
	m.Add("1.0", http.MethodPost, "/apps/{app}/run", AuthorizationRequiredHandler(runCommand))
	m.Add("1.0", http.MethodPost, "/apps/{app}/restart", AuthorizationRequiredHandler(restart))
	m.Add("1.0", http.MethodPost, "/apps/{app}/start", AuthorizationRequiredHandler(start))
	m.Add("1.0", http.MethodPost, "/apps/{app}/stop", AuthorizationRequiredHandler(stop))
	m.Add("1.0", http.MethodPost, "/apps/{app}/sleep", AuthorizationRequiredHandler(sleep))
	m.Add("1.10", http.MethodDelete, "/apps/{app}/versions/{version}", AuthorizationRequiredHandler(appVersionDelete))
	m.Add("1.0", http.MethodGet, "/apps/{app}/quota", AuthorizationRequiredHandler(getAppQuota))
	m.Add("1.0", http.MethodPut, "/apps/{app}/quota", AuthorizationRequiredHandler(changeAppQuota))
	m.Add("1.0", http.MethodGet, "/apps/{app}/env", AuthorizationRequiredHandler(getEnv))
	m.Add("1.0", http.MethodPost, "/apps/{app}/env", AuthorizationRequiredHandler(setEnv))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/env", AuthorizationRequiredHandler(unsetEnv))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/lock", AuthorizationRequiredHandler(forceDeleteLock))
	m.Add("1.0", http.MethodPut, "/apps/{app}/units", AuthorizationRequiredHandler(addUnits))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/units", AuthorizationRequiredHandler(removeUnits))
	m.Add("1.9", http.MethodGet, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(autoScaleUnitsInfo))
	m.Add("1.9", http.MethodPost, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(addAutoScaleUnits))
	m.Add("1.9", http.MethodDelete, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(removeAutoScaleUnits))
	m.Add("1.0", http.MethodPost, "/apps/{app}/units/register", AuthorizationRequiredHandler(registerUnit))
	m.Add("1.0", http.MethodPost, "/apps/{app}/units/{unit}", AuthorizationRequiredHandler(setUnitStatus))
	m.Add("1.0", http.MethodPut, "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(grantAppAccess))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(revokeAppAccess))
	m.AddNamed("log-get", "1.0", http.MethodGet, "/apps/{app}/log", AuthorizationRequiredHandler(appLog))
	m.AddNamed("log-get-instance", "1.8", http.MethodGet, "/apps/{app}/log-instance", AuthorizationRequiredHandler(appLog))
	m.Add("1.0", http.MethodPost, "/apps/{app}/log", AuthorizationRequiredHandler(addLog))
	m.Add("1.0", http.MethodPost, "/apps/{app}/deploy/rollback", AuthorizationRequiredHandler(deployRollback))
	m.Add("1.4", http.MethodPut, "/apps/{app}/deploy/rollback/update", AuthorizationRequiredHandler(deployRollbackUpdate))
	m.Add("1.3", http.MethodPost, "/apps/{app}/deploy/rebuild", AuthorizationRequiredHandler(deployRebuild))
	m.Add("1.0", http.MethodGet, "/apps/{app}/metric/envs", AuthorizationRequiredHandler(appMetricEnvs))
	m.Add("1.0", http.MethodPost, "/apps/{app}/routes", AuthorizationRequiredHandler(appRebuildRoutes))
	m.Add("1.2", http.MethodGet, "/apps/{app}/certificate", AuthorizationRequiredHandler(listCertificates))
	m.Add("1.2", http.MethodPut, "/apps/{app}/certificate", AuthorizationRequiredHandler(setCertificate))
	m.Add("1.2", http.MethodDelete, "/apps/{app}/certificate", AuthorizationRequiredHandler(unsetCertificate))

	m.Add("1.5", http.MethodPost, "/apps/{app}/routers", AuthorizationRequiredHandler(addAppRouter))
	m.Add("1.5", http.MethodPut, "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(updateAppRouter))
	m.Add("1.5", http.MethodDelete, "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(removeAppRouter))
	m.Add("1.5", http.MethodGet, "/apps/{app}/routers", AuthorizationRequiredHandler(listAppRouters))
	m.Add("1.8", http.MethodPost, "/apps/{app}/routable", AuthorizationRequiredHandler(appSetRoutable))

	m.Add("1.0", http.MethodPost, "/node/status", AuthorizationRequiredHandler(setNodeStatus))

	m.Add("1.0", http.MethodGet, "/deploys", AuthorizationRequiredHandler(deploysList))
	m.Add("1.0", http.MethodGet, "/deploys/{deploy}", AuthorizationRequiredHandler(deployInfo))

	m.Add("1.1", http.MethodGet, "/events", AuthorizationRequiredHandler(eventList))
	m.Add("1.3", http.MethodGet, "/events/blocks", AuthorizationRequiredHandler(eventBlockList))
	m.Add("1.3", http.MethodPost, "/events/blocks", AuthorizationRequiredHandler(eventBlockAdd))
	m.Add("1.3", http.MethodDelete, "/events/blocks/{uuid}", AuthorizationRequiredHandler(eventBlockRemove))
	m.Add("1.1", http.MethodGet, "/events/kinds", AuthorizationRequiredHandler(kindList))
	m.Add("1.1", http.MethodGet, "/events/{uuid}", AuthorizationRequiredHandler(eventInfo))
	m.Add("1.1", http.MethodPost, "/events/{uuid}/cancel", AuthorizationRequiredHandler(eventCancel))

	m.Add("1.6", http.MethodGet, "/events/webhooks", AuthorizationRequiredHandler(webhookList))
	m.Add("1.6", http.MethodPost, "/events/webhooks", AuthorizationRequiredHandler(webhookCreate))
	m.Add("1.6", http.MethodGet, "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookInfo))
	m.Add("1.6", http.MethodPut, "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookUpdate))
	m.Add("1.6", http.MethodDelete, "/events/webhooks/{name}", AuthorizationRequiredHandler(webhookDelete))

	m.Add("1.0", http.MethodGet, "/platforms", AuthorizationRequiredHandler(platformList))
	m.Add("1.0", http.MethodPost, "/platforms", AuthorizationRequiredHandler(platformAdd))
	m.Add("1.0", http.MethodPut, "/platforms/{name}", AuthorizationRequiredHandler(platformUpdate))
	m.Add("1.0", http.MethodDelete, "/platforms/{name}", AuthorizationRequiredHandler(platformRemove))
	m.Add("1.6", http.MethodGet, "/platforms/{name}", AuthorizationRequiredHandler(platformInfo))
	m.Add("1.6", http.MethodPost, "/platforms/{name}/rollback", AuthorizationRequiredHandler(platformRollback))

	// These handlers don't use {app} on purpose. Using :app means that only
	// the token generate for the given app is valid, but these handlers
	// use a token generated for Gandalf.
	m.Add("1.0", http.MethodPost, "/apps/{appname}/repository/clone", AuthorizationRequiredHandler(deploy))
	m.Add("1.0", http.MethodPost, "/apps/{appname}/deploy", AuthorizationRequiredHandler(deploy))
	m.Add("1.0", http.MethodPost, "/apps/{appname}/diff", AuthorizationRequiredHandler(diffDeploy))
	m.Add("1.5", http.MethodPost, "/apps/{appname}/build", AuthorizationRequiredHandler(build))

	// Shell also doesn't use {app} on purpose. Middlewares don't play well
	// with websocket.
	m.Add("1.0", http.MethodGet, "/apps/{appname}/shell", http.HandlerFunc(remoteShellHandler))

	m.Add("1.0", http.MethodGet, "/users", AuthorizationRequiredHandler(listUsers))
	m.Add("1.0", http.MethodPost, "/users", Handler(createUser))
	m.Add("1.0", http.MethodGet, "/users/info", AuthorizationRequiredHandler(userInfo))
	m.Add("1.0", http.MethodGet, "/auth/scheme", Handler(authScheme))
	m.Add("1.0", http.MethodPost, "/auth/login", Handler(login))

	m.Add("1.0", http.MethodPost, "/auth/saml", Handler(samlCallbackLogin))
	m.Add("1.0", http.MethodGet, "/auth/saml", Handler(samlMetadata))

	m.Add("1.0", http.MethodPost, "/users/{email}/password", Handler(resetPassword))
	m.Add("1.0", http.MethodPost, "/users/{email}/tokens", Handler(login))
	m.Add("1.0", http.MethodGet, "/users/{email}/quota", AuthorizationRequiredHandler(getUserQuota))
	m.Add("1.0", http.MethodPut, "/users/{email}/quota", AuthorizationRequiredHandler(changeUserQuota))
	m.Add("1.0", http.MethodDelete, "/users/tokens", AuthorizationRequiredHandler(logout))
	m.Add("1.0", http.MethodPut, "/users/password", AuthorizationRequiredHandler(changePassword))
	m.Add("1.0", http.MethodDelete, "/users", AuthorizationRequiredHandler(removeUser))
	m.Add("1.0", http.MethodGet, "/users/api-key", AuthorizationRequiredHandler(showAPIToken))
	m.Add("1.0", http.MethodPost, "/users/api-key", AuthorizationRequiredHandler(regenerateAPIToken))

	m.Add("1.0", http.MethodGet, "/logs", websocket.Handler(addLogs))

	m.Add("1.0", http.MethodGet, "/teams", AuthorizationRequiredHandler(teamList))
	m.Add("1.0", http.MethodPost, "/teams", AuthorizationRequiredHandler(createTeam))
	m.Add("1.0", http.MethodDelete, "/teams/{name}", AuthorizationRequiredHandler(removeTeam))
	m.Add("1.6", http.MethodPut, "/teams/{name}", AuthorizationRequiredHandler(updateTeam))
	m.Add("1.4", http.MethodGet, "/teams/{name}", AuthorizationRequiredHandler(teamInfo))
	m.Add("1.12", http.MethodGet, "/teams/{name}/quota", AuthorizationRequiredHandler(getTeamQuota))
	m.Add("1.12", http.MethodPut, "/teams/{name}/quota", AuthorizationRequiredHandler(changeTeamQuota))

	m.Add("1.0", http.MethodPost, "/swap", AuthorizationRequiredHandler(swap))

	m.Add("1.0", http.MethodGet, "/healthcheck/", http.HandlerFunc(healthcheck))
	m.Add("1.0", http.MethodGet, "/healthcheck", http.HandlerFunc(healthcheck))

	m.Add("1.0", http.MethodGet, "/iaas/machines", AuthorizationRequiredHandler(machinesList))
	m.Add("1.0", http.MethodDelete, "/iaas/machines/{machine_id}", AuthorizationRequiredHandler(machineDestroy))
	m.Add("1.0", http.MethodGet, "/iaas/templates", AuthorizationRequiredHandler(templatesList))
	m.Add("1.0", http.MethodPost, "/iaas/templates", AuthorizationRequiredHandler(templateCreate))
	m.Add("1.0", http.MethodPut, "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateUpdate))
	m.Add("1.0", http.MethodDelete, "/iaas/templates/{template_name}", AuthorizationRequiredHandler(templateDestroy))

	m.Add("1.0", http.MethodGet, "/plans", AuthorizationRequiredHandler(listPlans))
	m.Add("1.0", http.MethodPost, "/plans", AuthorizationRequiredHandler(addPlan))
	m.Add("1.0", http.MethodDelete, "/plans/{planname}", AuthorizationRequiredHandler(removePlan))

	m.Add("1.0", http.MethodGet, "/pools", AuthorizationRequiredHandler(poolList))
	m.Add("1.0", http.MethodPost, "/pools", AuthorizationRequiredHandler(addPoolHandler))
	m.Add("1.0", http.MethodDelete, "/pools/{name}", AuthorizationRequiredHandler(removePoolHandler))
	m.Add("1.0", http.MethodPut, "/pools/{name}", AuthorizationRequiredHandler(poolUpdateHandler))
	m.Add("1.0", http.MethodPost, "/pools/{name}/team", AuthorizationRequiredHandler(addTeamToPoolHandler))
	m.Add("1.0", http.MethodDelete, "/pools/{name}/team", AuthorizationRequiredHandler(removeTeamToPoolHandler))
	m.Add("1.8", http.MethodGet, "/pools/{name}", AuthorizationRequiredHandler(getPoolHandler))

	m.Add("1.3", http.MethodGet, "/constraints", AuthorizationRequiredHandler(poolConstraintList))
	m.Add("1.3", http.MethodPut, "/constraints", AuthorizationRequiredHandler(poolConstraintSet))

	m.Add("1.0", http.MethodGet, "/roles", AuthorizationRequiredHandler(listRoles))
	m.Add("1.4", http.MethodPut, "/roles", AuthorizationRequiredHandler(roleUpdate))
	m.Add("1.0", http.MethodPost, "/roles", AuthorizationRequiredHandler(addRole))
	m.Add("1.0", http.MethodGet, "/roles/{name}", AuthorizationRequiredHandler(roleInfo))
	m.Add("1.0", http.MethodDelete, "/roles/{name}", AuthorizationRequiredHandler(removeRole))
	m.Add("1.0", http.MethodPost, "/roles/{name}/permissions", AuthorizationRequiredHandler(addPermissions))
	m.Add("1.0", http.MethodDelete, "/roles/{name}/permissions/{permission}", AuthorizationRequiredHandler(removePermissions))
	m.Add("1.0", http.MethodPost, "/roles/{name}/user", AuthorizationRequiredHandler(assignRole))
	m.Add("1.0", http.MethodDelete, "/roles/{name}/user/{email}", AuthorizationRequiredHandler(dissociateRole))
	m.Add("1.0", http.MethodGet, "/role/default", AuthorizationRequiredHandler(listDefaultRoles))
	m.Add("1.0", http.MethodPost, "/role/default", AuthorizationRequiredHandler(addDefaultRole))
	m.Add("1.0", http.MethodDelete, "/role/default", AuthorizationRequiredHandler(removeDefaultRole))
	m.Add("1.0", http.MethodGet, "/permissions", AuthorizationRequiredHandler(listPermissions))
	m.Add("1.6", http.MethodPost, "/roles/{name}/token", AuthorizationRequiredHandler(assignRoleToToken))
	m.Add("1.6", http.MethodDelete, "/roles/{name}/token/{token_id}", AuthorizationRequiredHandler(dissociateRoleFromToken))
	m.Add("1.9", http.MethodPost, "/roles/{name}/group", AuthorizationRequiredHandler(assignRoleToGroup))
	m.Add("1.9", http.MethodDelete, "/roles/{name}/group/{group_name}", AuthorizationRequiredHandler(dissociateRoleFromGroup))

	m.Add("1.0", http.MethodGet, "/debug/goroutines", AuthorizationRequiredHandler(dumpGoroutines))
	m.Add("1.0", http.MethodGet, "/debug/pprof/", AuthorizationRequiredHandler(debugHandler(pprof.Index)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/cmdline", AuthorizationRequiredHandler(debugHandler(pprof.Cmdline)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/profile", AuthorizationRequiredHandler(debugHandler(pprof.Profile)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/symbol", AuthorizationRequiredHandler(debugHandler(pprof.Symbol)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/heap", AuthorizationRequiredHandler(debugHandler(pprof.Index)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/goroutine", AuthorizationRequiredHandler(debugHandler(pprof.Index)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/threadcreate", AuthorizationRequiredHandler(debugHandler(pprof.Index)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/block", AuthorizationRequiredHandler(debugHandler(pprof.Index)))
	m.Add("1.0", http.MethodGet, "/debug/pprof/trace", AuthorizationRequiredHandler(debugHandler(pprof.Trace)))
	m.Add("1.9", http.MethodGet, "/debug/fgprof", AuthorizationRequiredHandler(debugHandlerInt(fgprof.Handler())))

	m.Add("1.3", http.MethodGet, "/node/autoscale", AuthorizationRequiredHandler(autoScaleHistoryHandler))
	m.Add("1.3", http.MethodGet, "/node/autoscale/config", AuthorizationRequiredHandler(autoScaleGetConfig))
	m.Add("1.3", http.MethodPost, "/node/autoscale/run", AuthorizationRequiredHandler(autoScaleRunHandler))
	m.Add("1.3", http.MethodGet, "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleListRules))
	m.Add("1.3", http.MethodPost, "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleSetRule))
	m.Add("1.3", http.MethodDelete, "/node/autoscale/rules", AuthorizationRequiredHandler(autoScaleDeleteRule))
	m.Add("1.3", http.MethodDelete, "/node/autoscale/rules/{id}", AuthorizationRequiredHandler(autoScaleDeleteRule))

	m.Add("1.2", http.MethodGet, "/node", AuthorizationRequiredHandler(listNodesHandler))
	m.Add("1.2", http.MethodGet, "/node/apps/{appname}/containers", AuthorizationRequiredHandler(listUnitsByApp))
	m.Add("1.2", http.MethodGet, "/node/{address:.*}/containers", AuthorizationRequiredHandler(listUnitsByNode))
	m.Add("1.2", http.MethodPost, "/node", AuthorizationRequiredHandler(addNodeHandler))
	m.Add("1.2", http.MethodPut, "/node", AuthorizationRequiredHandler(updateNodeHandler))
	m.Add("1.2", http.MethodDelete, "/node/{address:.*}", AuthorizationRequiredHandler(removeNodeHandler))
	m.Add("1.3", http.MethodPost, "/node/rebalance", AuthorizationRequiredHandler(rebalanceNodesHandler))
	m.Add("1.6", http.MethodGet, "/node/{address:.*}", AuthorizationRequiredHandler(infoNodeHandler))

	m.Add("1.2", http.MethodGet, "/nodecontainers", AuthorizationRequiredHandler(nodeContainerList))
	m.Add("1.2", http.MethodPost, "/nodecontainers", AuthorizationRequiredHandler(nodeContainerCreate))
	m.Add("1.2", http.MethodGet, "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerInfo))
	m.Add("1.2", http.MethodDelete, "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerDelete))
	m.Add("1.2", http.MethodPost, "/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerUpdate))
	m.Add("1.2", http.MethodPost, "/nodecontainers/{name}/upgrade", AuthorizationRequiredHandler(nodeContainerUpgrade))

	m.Add("1.2", http.MethodPost, "/install/hosts", AuthorizationRequiredHandler(installHostAdd))
	m.Add("1.2", http.MethodGet, "/install/hosts", AuthorizationRequiredHandler(installHostList))
	m.Add("1.2", http.MethodGet, "/install/hosts/{name}", AuthorizationRequiredHandler(installHostInfo))

	m.Add("1.2", http.MethodGet, "/healing/node", AuthorizationRequiredHandler(nodeHealingRead))
	m.Add("1.2", http.MethodPost, "/healing/node", AuthorizationRequiredHandler(nodeHealingUpdate))
	m.Add("1.2", http.MethodDelete, "/healing/node", AuthorizationRequiredHandler(nodeHealingDelete))
	m.Add("1.3", http.MethodGet, "/healing", AuthorizationRequiredHandler(healingHistoryHandler))
	m.Add("1.3", http.MethodGet, "/routers", AuthorizationRequiredHandler(listRouters))
	m.Add("1.8", http.MethodPost, "/routers", AuthorizationRequiredHandler(addRouter))
	m.Add("1.8", http.MethodPut, "/routers/{name}", AuthorizationRequiredHandler(updateRouter))
	m.Add("1.8", http.MethodDelete, "/routers/{name}", AuthorizationRequiredHandler(deleteRouter))

	m.Add("1.2", http.MethodGet, "/metrics", promhttp.Handler())

	m.Add("1.7", http.MethodGet, "/provisioner", AuthorizationRequiredHandler(provisionerList))
	m.Add("1.3", http.MethodPost, "/provisioner/clusters", AuthorizationRequiredHandler(createCluster))
	m.Add("1.4", http.MethodPost, "/provisioner/clusters/{name}", AuthorizationRequiredHandler(updateCluster))
	m.Add("1.3", http.MethodGet, "/provisioner/clusters", AuthorizationRequiredHandler(listClusters))
	m.Add("1.8", http.MethodGet, "/provisioner/clusters/{name}", AuthorizationRequiredHandler(clusterInfo))
	m.Add("1.3", http.MethodDelete, "/provisioner/clusters/{name}", AuthorizationRequiredHandler(deleteCluster))

	m.Add("1.4", http.MethodGet, "/volumes", AuthorizationRequiredHandler(volumesList))
	m.Add("1.4", http.MethodPost, "/volumes", AuthorizationRequiredHandler(volumeCreate))
	m.Add("1.4", http.MethodGet, "/volumes/{name}", AuthorizationRequiredHandler(volumeInfo))
	m.Add("1.4", http.MethodPost, "/volumes/{name}", AuthorizationRequiredHandler(volumeUpdate))
	m.Add("1.4", http.MethodDelete, "/volumes/{name}", AuthorizationRequiredHandler(volumeDelete))
	m.Add("1.4", http.MethodPost, "/volumes/{name}/bind", AuthorizationRequiredHandler(volumeBind))
	m.Add("1.4", http.MethodDelete, "/volumes/{name}/bind", AuthorizationRequiredHandler(volumeUnbind))
	m.Add("1.4", http.MethodGet, "/volumeplans", AuthorizationRequiredHandler(volumePlansList))

	m.Add("1.6", http.MethodGet, "/tokens", AuthorizationRequiredHandler(tokenList))
	m.Add("1.7", http.MethodGet, "/tokens/{token_id}", AuthorizationRequiredHandler(tokenInfo))
	m.Add("1.6", http.MethodPost, "/tokens", AuthorizationRequiredHandler(tokenCreate))
	m.Add("1.6", http.MethodDelete, "/tokens/{token_id}", AuthorizationRequiredHandler(tokenDelete))
	m.Add("1.6", http.MethodPut, "/tokens/{token_id}", AuthorizationRequiredHandler(tokenUpdate))

	m.Add("1.7", http.MethodGet, "/brokers", AuthorizationRequiredHandler(serviceBrokerList))
	m.Add("1.7", http.MethodPost, "/brokers", AuthorizationRequiredHandler(serviceBrokerAdd))
	m.Add("1.7", http.MethodPut, "/brokers/{broker}", AuthorizationRequiredHandler(serviceBrokerUpdate))
	m.Add("1.7", http.MethodDelete, "/brokers/{broker}", AuthorizationRequiredHandler(serviceBrokerDelete))

	// Handlers for compatibility reasons, should be removed on tsuru 2.0.
	m.Add("1.4", http.MethodPost, "/teams/{name}", AuthorizationRequiredHandler(updateTeam))
	m.Add("1.0", http.MethodGet, "/docker/node", AuthorizationRequiredHandler(listNodesHandler))
	m.Add("1.0", http.MethodGet, "/docker/node/apps/{appname}/containers", AuthorizationRequiredHandler(listUnitsByApp))
	m.Add("1.0", http.MethodGet, "/docker/node/{address:.*}/containers", AuthorizationRequiredHandler(listUnitsByNode))
	m.Add("1.0", http.MethodPost, "/docker/node", AuthorizationRequiredHandler(addNodeHandler))
	m.Add("1.0", http.MethodPut, "/docker/node", AuthorizationRequiredHandler(updateNodeHandler))
	m.Add("1.0", http.MethodDelete, "/docker/node/{address:.*}", AuthorizationRequiredHandler(removeNodeHandler))
	m.Add("1.0", http.MethodPost, "/docker/containers/rebalance", AuthorizationRequiredHandler(rebalanceNodesHandler))

	m.Add("1.0", http.MethodGet, "/docker/nodecontainers", AuthorizationRequiredHandler(nodeContainerList))
	m.Add("1.0", http.MethodPost, "/docker/nodecontainers", AuthorizationRequiredHandler(nodeContainerCreate))
	m.Add("1.0", http.MethodGet, "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerInfo))
	m.Add("1.0", http.MethodDelete, "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerDelete))
	m.Add("1.0", http.MethodPost, "/docker/nodecontainers/{name}", AuthorizationRequiredHandler(nodeContainerUpdate))
	m.Add("1.0", http.MethodPost, "/docker/nodecontainers/{name}/upgrade", AuthorizationRequiredHandler(nodeContainerUpgrade))

	m.Add("1.0", http.MethodGet, "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingRead))
	m.Add("1.0", http.MethodPost, "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingUpdate))
	m.Add("1.0", http.MethodDelete, "/docker/healing/node", AuthorizationRequiredHandler(nodeHealingDelete))
	m.Add("1.0", http.MethodGet, "/docker/healing", AuthorizationRequiredHandler(healingHistoryHandler))

	m.Add("1.0", http.MethodGet, "/docker/autoscale", AuthorizationRequiredHandler(autoScaleHistoryHandler))
	m.Add("1.0", http.MethodGet, "/docker/autoscale/config", AuthorizationRequiredHandler(autoScaleGetConfig))
	m.Add("1.0", http.MethodPost, "/docker/autoscale/run", AuthorizationRequiredHandler(autoScaleRunHandler))
	m.Add("1.0", http.MethodGet, "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleListRules))
	m.Add("1.0", http.MethodPost, "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleSetRule))
	m.Add("1.0", http.MethodDelete, "/docker/autoscale/rules", AuthorizationRequiredHandler(autoScaleDeleteRule))
	m.Add("1.0", http.MethodDelete, "/docker/autoscale/rules/{id}", AuthorizationRequiredHandler(autoScaleDeleteRule))

	m.Add("1.0", http.MethodGet, "/plans/routers", AuthorizationRequiredHandler(listRouters))

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(negroni.HandlerFunc(contextClearerMiddleware))
	n.Use(negroni.HandlerFunc(contextNoCancelMiddleware))
	if !dry {
		n.Use(observability.NewMiddleware())
	}
	n.UseHandler(m)
	n.Use(&flushingWriterMiddleware{
		latencyConfig: map[string]time.Duration{
			"log-get":          500 * time.Millisecond,
			"log-get-instance": 500 * time.Millisecond,
		},
	})
	n.Use(negroni.HandlerFunc(setRequestIDHeaderMiddleware))
	n.Use(negroni.HandlerFunc(errorHandlingMiddleware))
	n.Use(negroni.HandlerFunc(setVersionHeadersMiddleware))
	n.Use(negroni.HandlerFunc(authTokenMiddleware))
	n.UseHandler(http.HandlerFunc(runDelayedHandler))

	form.DefaultEncoder = form.DefaultEncoder.UseJSONTags(false)

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
		fmt.Fprintln(os.Stderr, "Warning: configuration didn't declare a database driver, using default driver.")
	}
	fmt.Fprintf(os.Stderr, "Using %q database %q from the server %q.\n", dbDriverName, dbName, connString)
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
	a, err := app.GetByName(context.TODO(), appName)
	if err == appTypes.ErrAppNotFound {
		return nil, nil
	}
	return a, err
}

func bindAppsLister() ([]bind.App, error) {
	apps, err := app.List(context.TODO(), nil)
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
	span, ctx := opentracing.StartSpanFromContext(
		context.Background(), "StartServer")
	defer span.Finish()

	srvConf, err := createServers(handler)
	if err != nil {
		return err
	}
	shutdownTimeoutInt, _ := config.GetInt("shutdown-timeout")
	srvConf.shutdownTimeout = 10 * time.Minute
	if shutdownTimeoutInt != 0 {
		srvConf.shutdownTimeout = time.Duration(shutdownTimeoutInt) * time.Second
	}
	go srvConf.handleSignals(srvConf.shutdownTimeout)

	defer srvConf.shutdown(srvConf.shutdownTimeout)

	shutdown.Register(&logTracker)
	var startupMessage string
	err = router.Initialize()
	if err != nil {
		return err
	}
	routers, err := router.List(ctx)
	if err != nil {
		return err
	}
	for _, routerDesc := range routers {
		var r router.Router
		r, err = router.Get(ctx, routerDesc.Name)
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
	defaultRouter, _ := router.Default(ctx)
	fmt.Printf("Default router is %q.\n", defaultRouter)
	err = rebuild.Initialize(appFinder)
	if err != nil {
		return err
	}
	scheme, err := getAuthScheme()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: configuration didn't declare auth:scheme, using default scheme.")
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	fmt.Printf("Using %q auth scheme.\n", scheme)
	_, err = nodecontainer.InitializeBS(ctx, app.AuthScheme, app.InternalAppName)
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
	results := hc.Check(ctx, "all")
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
	useTLS, _ := config.GetBool("use-tls")
	tlsListen, _ := config.GetString("tls:listen")
	listen, _ := config.GetString("listen")
	if useTLS {
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
		srvConf.validateCertificate, _ = config.GetBool("tls:validate-certificate")
		if _, err := srvConf.loadCertificate(); err != nil {
			return nil, err
		}
		if srvConf.validateCertificate {
			certValidator := &certificateValidator{
				conf: &srvConf,
			}
			shutdown.Register(certValidator)
			certValidator.start()
		}
		autoReloadInterval, err := config.GetDuration("tls:auto-reload:interval")
		if err == nil && autoReloadInterval > 0 {
			reloader := &certificateReloader{
				conf:     &srvConf,
				interval: autoReloadInterval,
			}
			shutdown.Register(reloader)
			reloader.start()
			srvConf.certificateReloadedCh = make(chan bool)
		}
		srvConf.httpsSrv = &http.Server{
			ReadTimeout:  time.Duration(readTimeout) * time.Second,
			WriteTimeout: time.Duration(writeTimeout) * time.Second,
			Addr:         tlsListen,
			Handler:      handler,
			TLSConfig: &tls.Config{
				GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
					srvConf.Lock()
					defer srvConf.Unlock()
					if srvConf.certificate == nil {
						return nil, errors.New("there are no certificates to offer")
					}
					return srvConf.certificate, nil
				},
			},
		}
	}
	return &srvConf, nil
}

type certificateReloader struct {
	conf     *srvConfig
	interval time.Duration
	stopCh   chan bool
	once     *sync.Once
}

func (cr *certificateReloader) Shutdown(ctx context.Context) error {
	if cr.stopCh == nil {
		return nil
	}
	close(cr.stopCh)
	return nil
}

func (cr *certificateReloader) start() {
	if cr.once == nil {
		cr.once = &sync.Once{}
	}
	cr.once.Do(func() {
		cr.stopCh = make(chan bool)
		go func() {
			for {
				log.Debugf("[certificate-reloader] starting the certificate reloader")
				changed, err := cr.conf.loadCertificate()
				if err != nil {
					log.Errorf("[certificate-reloader] error when reloading a certificate: %v\n", err)
				}
				if changed {
					fmt.Println("[certificate-reloader] a new certificate was successfully loaded")
					cr.conf.certificateReloadedCh <- true
				}
				log.Debugf("[certificate-reloader] finishing the certificate reloader")
				select {
				case <-cr.stopCh:
					return
				case <-time.After(cr.interval):
				}
			}
		}()
	})
}

type certificateValidator struct {
	conf       *srvConfig
	stopCh     chan bool
	stopDoneCh chan bool
	once       *sync.Once
	// shutdownServerFunc points to a function which is called whenever the
	// certificates become invalid. If not defined, its default action is
	// gracefully shutdown the server.
	shutdownServerFunc func(error)
}

func (cv *certificateValidator) Shutdown(ctx context.Context) error {
	if cv.stopCh == nil {
		return nil
	}
	close(cv.stopCh)
	select {
	case <-cv.stopDoneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (cv *certificateValidator) start() {
	if cv.once == nil {
		cv.once = &sync.Once{}
	}
	cv.once.Do(func() {
		cv.stopCh = make(chan bool)
		cv.stopDoneCh = make(chan bool)
		if cv.shutdownServerFunc == nil {
			cv.shutdownServerFunc = func(err error) {
				cv.conf.shutdown(cv.conf.shutdownTimeout)
			}
		}
		go func() {
			for {
				log.Debug("[certificate-validator] starting certificate validator")
				cv.conf.Lock()
				certificate, err := x509.ParseCertificate(cv.conf.certificate.Certificate[0])
				if err != nil {
					log.Errorf("[certificate-validator] could not parse the current certificate as a x509 certificate: %v\n", err)
					time.Sleep(time.Second)
					cv.conf.Unlock()
					continue
				}
				nextValidation := time.Until(certificate.NotAfter)
				err = validateTLSCertificate(cv.conf.certificate, cv.conf.roots)
				cv.conf.Unlock()
				if err != nil {
					log.Errorf("[certificate-validator] the currently loaded certificate is invalid: %v\n", err)
					cv.shutdownServerFunc(err)
				} else {
					fmt.Printf("[certificate-validator] certificate is valid, next validation scheduled to %s\n", nextValidation)
				}
				log.Debug("[certificate-validator] finishing certificate validator")
				select {
				case <-time.After(nextValidation):
				case <-cv.conf.certificateReloadedCh:
					continue
				case <-cv.stopCh:
					cv.stopDoneCh <- true
					return
				}
			}
		}()
	})
}

// validateTLSCertificate checks if c is ready for use in a production env. When
// c is not a good one, returns a non-nil error describing the problem, otherwise
// returns nil indicating success.
//
// A good certificate should be issued (even though indirectly, when providing
// intermediates) by roots; within the issuing time boundary; and, match the
// Common Name or SAN extension with the server's hostname (defined by "host"
// entry in the API config file). See x509.Verify to more detailed info.
func validateTLSCertificate(c *tls.Certificate, roots *x509.CertPool) error {
	configHost, err := config.GetString("host")
	if err != nil {
		return err
	}
	urlHost, err := url.Parse(configHost)
	if err != nil {
		return err
	}
	hostname := urlHost.Hostname()
	if c == nil || len(c.Certificate) == 0 {
		return errors.New("there is no certificate provided")
	}
	var intermediateCertPool *x509.CertPool
	if len(c.Certificate) > 1 {
		intermediateCertPool = x509.NewCertPool()
		for i := 1; i < len(c.Certificate); i++ {
			var intermerdiateCert *x509.Certificate
			if intermerdiateCert, err = x509.ParseCertificate(c.Certificate[i]); err != nil {
				return err
			}
			intermediateCertPool.AddCert(intermerdiateCert)
		}
	}
	leafCert, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return err
	}
	_, err = leafCert.Verify(x509.VerifyOptions{
		DNSName:       hostname,
		Roots:         roots,
		Intermediates: intermediateCertPool,
	})
	return err
}

type srvConfig struct {
	certFile        string
	keyFile         string
	httpSrv         *http.Server
	httpsSrv        *http.Server
	certificate     *tls.Certificate
	shutdownTimeout time.Duration
	// roots holds a set of trusted certificates that are used by certificate
	// validator to check a given certificate. If roots is nil, the system
	// certificates are used instead.
	roots *x509.CertPool
	// certificateReloadedCh indicates when new certificates were reloaded by
	// certificate reloader routine.
	certificateReloadedCh chan bool
	once                  sync.Once
	sync.Mutex
	shutdownCalled      bool
	validateCertificate bool
}

func (conf *srvConfig) loadCertificate() (bool, error) {
	conf.Lock()
	defer conf.Unlock()
	newCertificate, err := tls.LoadX509KeyPair(conf.certFile, conf.keyFile)
	if err != nil {
		return false, err
	}
	if conf.validateCertificate {
		if err = validateTLSCertificate(&newCertificate, conf.roots); err != nil {
			return false, err
		}
	}
	if conf.certificate == nil {
		conf.certificate = &newCertificate
		return true, nil
	}
	if len(newCertificate.Certificate) != len(conf.certificate.Certificate) {
		conf.certificate = &newCertificate
		return true, nil
	}
	for i := 0; i < len(newCertificate.Certificate); i++ {
		newer, err := x509.ParseCertificate(newCertificate.Certificate[i])
		if err != nil {
			return false, err
		}
		older, _ := x509.ParseCertificate(conf.certificate.Certificate[i])
		if !older.Equal(newer) {
			conf.certificate = &newCertificate
			return true, nil
		}
	}
	return false, nil
}

func (conf *srvConfig) shutdown(shutdownTimeout time.Duration) {
	conf.Lock()
	defer conf.Unlock()
	conf.once.Do(func() {
		conf.onceShutdown(shutdownTimeout)
	})
	conf.shutdownCalled = true
}

func (conf *srvConfig) onceShutdown(shutdownTimeout time.Duration) {
	var wg sync.WaitGroup
	defer wg.Wait()
	shutdownSrv := func(srv *http.Server) {
		defer wg.Done()
		fmt.Printf("[shutdown] tsuru is shutting down server %v, waiting for pending connections to finish.\n", srv.Addr)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		err := srv.Shutdown(ctx)
		if err != nil {
			fmt.Printf("[shutdown] error while shutting down server %v: %v\n", srv.Addr, err)
		}
	}
	if conf.httpSrv != nil {
		wg.Add(1)
		go shutdownSrv(conf.httpSrv)
	}
	if conf.httpsSrv != nil {
		wg.Add(1)
		go shutdownSrv(conf.httpsSrv)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("[shutdown] tsuru is running shutdown handlers")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		shutdown.Do(ctx, os.Stdout)
		cancel()
	}()
}

func (conf *srvConfig) handleSignals(shutdownTimeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	conf.shutdown(shutdownTimeout)
}

func (conf *srvConfig) start() <-chan error {
	conf.Lock()
	defer conf.Unlock()
	errChan := make(chan error, 2)
	if conf.shutdownCalled {
		errChan <- errors.New("shutdown called")
		return errChan
	}
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
