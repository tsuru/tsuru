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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cezarsa/form"
	"github.com/codegangsta/negroni"
	"github.com/felixge/fgprof"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/observability"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/api/tracker"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/app/image/gc"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/applog"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/multi"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	_ "github.com/tsuru/tsuru/auth/oidc"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/webhook"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/tag"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/volume"
	"go.opentelemetry.io/otel"
	"golang.org/x/net/websocket"
)

var (
	Version = "1.23.1"
	GitHash = ""
)

type TsuruHandler struct {
	version string
	method  string
	path    string
	h       http.Handler
}

func fatal(err error) {
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
		return errors.Wrapf(err, "could not initialize app service")
	}
	servicemanager.TeamToken, err = auth.TeamTokenService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize team token service")
	}
	servicemanager.AppCache, err = app.CacheService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize app cache service")
	}
	servicemanager.Team, err = auth.TeamService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize team service")
	}
	servicemanager.Plan, err = app.PlanService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize plan service")
	}
	servicemanager.Platform, err = app.PlatformService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize platform service")
	}
	servicemanager.PlatformImage, err = image.PlatformImageService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize platform image service")
	}
	servicemanager.UserQuota, err = auth.UserQuotaService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize user quota service")
	}
	servicemanager.AppQuota, err = app.QuotaService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize quota service")
	}
	servicemanager.TeamQuota, err = auth.TeamQuotaService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize team quota service")
	}
	servicemanager.Webhook, err = webhook.WebhookService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize webhook service")
	}
	servicemanager.Cluster, err = cluster.ClusterService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize cluster service")
	}
	servicemanager.LogService, err = applog.AppLogService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize app log service")
	}
	servicemanager.InstanceTracker, err = tracker.InstanceService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize app tracker service")
	}
	servicemanager.DynamicRouter, err = router.DynamicRouterService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize app dynamic router service")
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize app version service")
	}
	servicemanager.AuthGroup, err = auth.GroupService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize auth group service")
	}
	servicemanager.Pool, err = pool.PoolService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize pool service")
	}
	servicemanager.Volume, err = volume.VolumeService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize volume service")
	}
	servicemanager.Job, err = job.JobService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize job service")
	}
	servicemanager.Tag, err = tag.TagService()
	if err != nil {
		return errors.Wrapf(err, "could not initialize tag service")
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
	m.Add("1.13", http.MethodPut, "/services/{service}/instances/{instance}/apps/{app}", AuthorizationRequiredHandler(bindServiceInstance))
	m.Add("1.13", http.MethodDelete, "/services/{service}/instances/{instance}/apps/{app}", AuthorizationRequiredHandler(unbindServiceInstance))
	m.Add("1.13", http.MethodPut, "/services/{service}/instances/{instance}/jobs/{job}", AuthorizationRequiredHandler(bindJobServiceInstance))
	m.Add("1.13", http.MethodDelete, "/services/{service}/instances/{instance}/jobs/{job}", AuthorizationRequiredHandler(unbindJobServiceInstance))

	m.Add("1.0", http.MethodPut, "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceGrantTeam))
	m.Add("1.0", http.MethodDelete, "/services/{service}/instances/permission/{instance}/{team}", AuthorizationRequiredHandler(serviceInstanceRevokeTeam))

	m.AddAll("1.0", "/services/{service}/proxy/{instance}", AuthorizationRequiredHandler(serviceInstanceProxy))
	m.AddAll("1.20", "/services/{service}/resources/{instance}/{path:.*}", AuthorizationRequiredHandler(serviceInstanceProxyV2))
	m.AddAll("1.0", "/services/proxy/service/{service}", AuthorizationRequiredHandler(serviceProxy))
	m.AddAll("1.21", "/services/{service}/authenticated-resources/{path:.*}", AuthorizationRequiredHandler(serviceAuthenticatedResourcesProxy))

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
	m.Add("1.10", http.MethodDelete, "/apps/{app}/versions/{version}", AuthorizationRequiredHandler(appVersionDelete))
	m.Add("1.0", http.MethodGet, "/apps/{app}/quota", AuthorizationRequiredHandler(getAppQuota))
	m.Add("1.0", http.MethodPut, "/apps/{app}/quota", AuthorizationRequiredHandler(changeAppQuota))
	m.Add("1.0", http.MethodGet, "/apps/{app}/env", AuthorizationRequiredHandler(getAppEnv))
	m.Add("1.0", http.MethodPost, "/apps/{app}/env", AuthorizationRequiredHandler(setAppEnv))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/env", AuthorizationRequiredHandler(unsetAppEnv))
	m.Add("1.0", http.MethodPut, "/apps/{app}/units", AuthorizationRequiredHandler(addUnits))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/units", AuthorizationRequiredHandler(removeUnits))
	m.Add("1.9", http.MethodGet, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(autoScaleUnitsInfo))
	m.Add("1.9", http.MethodPost, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(addAutoScaleUnits))
	m.Add("1.29", http.MethodPost, "/apps/{app}/units/autoscale/swap", AuthorizationRequiredHandler(swapAutoScaleUnits))
	m.Add("1.9", http.MethodDelete, "/apps/{app}/units/autoscale", AuthorizationRequiredHandler(removeAutoScaleUnits))
	m.Add("1.12", http.MethodDelete, "/apps/{app}/units/{unit}", AuthorizationRequiredHandler(killUnit))
	m.Add("1.0", http.MethodPut, "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(grantAppAccess))
	m.Add("1.0", http.MethodDelete, "/apps/{app}/teams/{team}", AuthorizationRequiredHandler(revokeAppAccess))
	m.AddNamed("log-get", "1.0", http.MethodGet, "/apps/{app}/log", AuthorizationRequiredHandler(appLog))
	m.AddNamed("log-get-instance", "1.8", http.MethodGet, "/apps/{app}/log-instance", AuthorizationRequiredHandler(appLog))
	m.Add("1.0", http.MethodPost, "/apps/{app}/log", AuthorizationRequiredHandler(addLog))
	m.Add("1.0", http.MethodPost, "/apps/{app}/deploy/rollback", AuthorizationRequiredHandler(deployRollback))
	m.Add("1.4", http.MethodPut, "/apps/{app}/deploy/rollback/update", AuthorizationRequiredHandler(deployRollbackUpdate))
	m.Add("1.3", http.MethodPost, "/apps/{app}/deploy/rebuild", AuthorizationRequiredHandler(deployRebuild))
	m.Add("1.0", http.MethodPost, "/apps/{app}/routes", AuthorizationRequiredHandler(appRebuildRoutes))

	m.Add("1.2", http.MethodGet, "/apps/{app}/certificate", AuthorizationRequiredHandler(listCertificatesLegacy))
	m.Add("1.24", http.MethodGet, "/apps/{app}/certificate", AuthorizationRequiredHandler(listCertificates))

	m.Add("1.2", http.MethodPut, "/apps/{app}/certificate", AuthorizationRequiredHandler(setCertificate))
	m.Add("1.2", http.MethodDelete, "/apps/{app}/certificate", AuthorizationRequiredHandler(unsetCertificate))
	m.Add("1.24", http.MethodPut, "/apps/{app}/certissuer", AuthorizationRequiredHandler(setCertIssuer))
	m.Add("1.24", http.MethodDelete, "/apps/{app}/certissuer", AuthorizationRequiredHandler(unsetCertIssuer))

	m.Add("1.5", http.MethodPost, "/apps/{app}/routers", AuthorizationRequiredHandler(addAppRouter))
	m.Add("1.5", http.MethodPut, "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(updateAppRouter))
	m.Add("1.5", http.MethodDelete, "/apps/{app}/routers/{router}", AuthorizationRequiredHandler(removeAppRouter))
	m.Add("1.5", http.MethodGet, "/apps/{app}/routers", AuthorizationRequiredHandler(listAppRouters))
	m.Add("1.8", http.MethodPost, "/apps/{app}/routable", AuthorizationRequiredHandler(appSetRoutable))
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
	m.Add("1.18", http.MethodGet, "/auth/schemes", Handler(authSchemes))
	m.Add("1.0", http.MethodPost, "/auth/login", Handler(login))

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
	m.Add("1.17", http.MethodGet, "/teams/{name}/users", AuthorizationRequiredHandler(teamUserList))
	m.Add("1.17", http.MethodGet, "/teams/{name}/groups", AuthorizationRequiredHandler(teamGroupList))

	m.Add("1.0", http.MethodPost, "/swap", AuthorizationRequiredHandler(swap))

	m.Add("1.0", http.MethodGet, "/healthcheck/", http.HandlerFunc(healthcheck))
	m.Add("1.0", http.MethodGet, "/healthcheck", http.HandlerFunc(healthcheck))

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

	// Handlers for compatibility reasons, should be removed on tsuru 2.0.
	m.Add("1.4", http.MethodPost, "/teams/{name}", AuthorizationRequiredHandler(updateTeam))

	m.Add("1.0", http.MethodGet, "/plans/routers", AuthorizationRequiredHandler(listRouters))

	m.Add("1.13", http.MethodPost, "/jobs", AuthorizationRequiredHandler(createJob))
	m.Add("1.13", http.MethodPost, "/jobs/{name}/trigger", AuthorizationRequiredHandler(jobTrigger))
	m.Add("1.13", http.MethodGet, "/jobs/{name}", AuthorizationRequiredHandler(jobInfo))
	m.Add("1.13", http.MethodDelete, "/jobs/{name}", AuthorizationRequiredHandler(deleteJob))
	m.Add("1.13", http.MethodPut, "/jobs/{name}", AuthorizationRequiredHandler(updateJob))
	m.Add("1.13", http.MethodGet, "/jobs", AuthorizationRequiredHandler(jobList))
	m.Add("1.16", http.MethodGet, "/jobs/{name}/env", AuthorizationRequiredHandler(getJobEnv))
	m.Add("1.13", http.MethodPost, "/jobs/{name}/env", AuthorizationRequiredHandler(setJobEnv))
	m.Add("1.13", http.MethodDelete, "/jobs/{name}/env", AuthorizationRequiredHandler(unsetJobEnv))
	m.Add("1.13", http.MethodGet, "/jobs/{name}/log", AuthorizationRequiredHandler(jobLog))
	m.Add("1.13", http.MethodDelete, "/jobs/{name}/units/{unit}", AuthorizationRequiredHandler(killJob))
	m.Add("1.23", http.MethodPost, "/jobs/{name}/deploy", AuthorizationRequiredHandler(jobDeploy))

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	if c := corsMiddleware(); c != nil {
		stdLog.Printf("Cors enabled")
		n.Use(c)
	}
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
	dbDriverName, err := config.GetString("database:driver")
	if err != nil {
		dbDriverName = storage.DefaultDbDriverName
		log.Debugf("Warning: configuration didn't declare a database driver, using default driver.")
	}
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

func appFinder(appName string) (*appTypes.App, error) {
	a, err := app.GetByName(context.TODO(), appName)
	if err == appTypes.ErrAppNotFound {
		return nil, nil
	}
	return a, err
}

func startServer(handler http.Handler) error {
	tracer := otel.Tracer("tsuru/api")
	ctx, span := tracer.Start(context.Background(), "StartServer")
	defer span.End()

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
	routers, err := router.List(ctx)
	if err != nil {
		return err
	}
	for _, routerDesc := range routers {
		_, err = router.Get(ctx, routerDesc.Name)
		if err != nil {
			return err
		}
		log.Debugf("Registered router %q", routerDesc.Name)
	}
	rebuild.Initialize(appFinder)
	scheme, err := getAuthScheme()
	if err != nil {
		log.Debugf("Warning: configuration didn't declare auth:scheme, using default scheme.")
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	log.Debugf("Using %q auth scheme.", scheme)
	err = provision.InitializeAll()
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
	log.Debugf("Checking components status:")
	results := hc.Check(ctx, "all")
	for _, result := range results {
		if result.Status != hc.HealthCheckOK {
			log.Errorf("WARNING: %q is not working: %s", result.Name, result.Status)
		}
	}
	log.Debugf("Components checked.")

	err = <-srvConf.start()
	if err != http.ErrServerClosed {
		return errors.Wrap(err, "unexpected error in server while listening")
	}
	log.Debugf("Listening stopped: %s", err)
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
		if _, err := srvConf.readCertificateFromFilesystem(); err != nil {
			return nil, err
		}
		if srvConf.validateCertificate {
			certValidator := &certificateValidator{
				conf: &srvConf,
			}
			shutdown.Register(certValidator)
			certValidator.start()
		}

		srvConf.certificateReloadedCh = make(chan bool)

		reloader := &certificateReloader{
			conf: &srvConf,
		}
		shutdown.Register(reloader)
		reloader.start()

		srvConf.httpsSrv = &http.Server{
			ReadTimeout:  time.Duration(readTimeout) * time.Second,
			WriteTimeout: time.Duration(writeTimeout) * time.Second,
			Addr:         tlsListen,
			Handler:      handler,
			TLSConfig: &tls.Config{
				GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
					cert := srvConf.certificate.Load()

					if cert == nil {
						return nil, errors.New("there are no certificates to offer")
					}
					return cert, nil
				},
			},
		}
	}
	return &srvConf, nil
}

type certificateReloader struct {
	conf   *srvConfig
	stopCh chan bool
	once   *sync.Once
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
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Debugf("[certificate-reloader] could not initialize watcher: %s", err.Error())
				return
			}

			defer watcher.Close()

			// Add path of cert
			err = watcher.Add(cr.conf.certFile)
			if err != nil {
				log.Debugf("[certificate-reloader] could watch cert file, err: %s\n", err.Error())
				return
			}

			// Add path of cert
			err = watcher.Add(cr.conf.keyFile)
			if err != nil {
				log.Debugf("[certificate-reloader] could watch key file, err: %s\n", err.Error())
				return
			}

			for {
				select {
				case <-cr.stopCh:
					return
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}

					forceReload := false

					if event.Has(fsnotify.Remove) {
						// k8s configmaps uses symlinks, we need this workaround.
						// original configmap file is removed
						watcher.Remove(event.Name)
						watcher.Add(event.Name)
						forceReload = true
					}

					if event.Has(fsnotify.Write) || forceReload {
						log.Debugf("[certificate-reloader] modified file: %s\n", event.Name)

						changed, err := cr.conf.readCertificateFromFilesystem()
						if err != nil {
							log.Errorf("[certificate-reloader] error when reloading a certificate: %v\n", err)
						}
						if changed {
							log.Debugf("[certificate-reloader] a new certificate was successfully loaded")

							// send message to certificateReloadedCh without blocking
							select {
							case cr.conf.certificateReloadedCh <- true:
							default:
							}
						}
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Errorf("[certificate-reloader] error: %s", err.Error())
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
				certificate, err := x509.ParseCertificate(cv.conf.certificate.Load().Certificate[0])
				if err != nil {
					log.Errorf("[certificate-validator] could not parse the current certificate as a x509 certificate: %v\n", err)
					time.Sleep(time.Second)
					continue
				}
				nextValidation := time.Until(certificate.NotAfter)
				err = validateTLSCertificate(cv.conf.certificate.Load(), cv.conf.roots)
				if err != nil {
					log.Errorf("[certificate-validator] the currently loaded certificate is invalid: %v\n", err)
					cv.shutdownServerFunc(err)
				} else {
					log.Debugf("[certificate-validator] certificate is valid, next validation scheduled to %s", nextValidation)
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
	certificate     atomic.Pointer[tls.Certificate]
	shutdownTimeout time.Duration
	// roots holds a set of trusted certificates that are used by certificate
	// validator to check a given certificate. If roots is nil, the system
	// certificates are used instead.
	roots *x509.CertPool
	// certificateReloadedCh indicates when new certificates were reloaded by
	// certificate reloader routine.
	certificateReloadedCh chan bool
	once                  sync.Once
	shutdownCalled        bool
	validateCertificate   bool
}

func (conf *srvConfig) readCertificateFromFilesystem() (changed bool, err error) {
	newCertificate, err := tls.LoadX509KeyPair(conf.certFile, conf.keyFile)
	if err != nil {
		return false, err
	}
	if conf.validateCertificate {
		if err = validateTLSCertificate(&newCertificate, conf.roots); err != nil {
			return false, err
		}
	}
	if conf.certificate.Load() == nil {
		conf.certificate.Store(&newCertificate)
		return true, nil
	}
	if len(newCertificate.Certificate) != len(conf.certificate.Load().Certificate) {
		conf.certificate.Store(&newCertificate)
		return true, nil
	}
	for i := 0; i < len(newCertificate.Certificate); i++ {
		newer, err := x509.ParseCertificate(newCertificate.Certificate[i])
		if err != nil {
			return false, err
		}
		older, _ := x509.ParseCertificate(conf.certificate.Load().Certificate[i])
		if !older.Equal(newer) {
			conf.certificate.Store(&newCertificate)
			return true, nil
		}
	}
	return false, nil
}

func (conf *srvConfig) shutdown(shutdownTimeout time.Duration) {
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
