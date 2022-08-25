// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/registry"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/cache"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	routerTypes "github.com/tsuru/tsuru/types/router"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	"github.com/tsuru/tsuru/validation"
)

var AuthScheme auth.Scheme

var (
	ErrAlreadyHaveAccess = errors.New("team already have access to this app")
	ErrNoAccess          = errors.New("team does not have access to this app")
	ErrCannotOrphanApp   = errors.New("cannot revoke access from this team, as it's the unique team with access to the app")
	ErrDisabledPlatform  = errors.New("Disabled Platform, only admin users can create applications with the platform")

	ErrRouterAlreadyLinked = errors.New("router already linked to this app")

	ErrNoVersionProvisioner = errors.New("The current app provisioner does not support multiple versions handling")
	ErrSwapMultipleVersions = errors.New("swapping apps with multiple versions is not allowed")
	ErrSwapMultipleRouters  = errors.New("swapping apps with multiple routers is not supported")
	ErrSwapDifferentRouters = errors.New("swapping apps with different routers is not supported")
	ErrSwapNoCNames         = errors.New("no cnames to swap")
	ErrSwapDeprecated       = errors.New("swapping using router api v2 will work only with cnameOnly")
)

var (
	counterNodesNotFound = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_node_status_not_found",
		Help: "The number of not found nodes received in tsuru node status.",
	})
	envVarNameRegexp = regexp.MustCompile("^[a-zA-Z][-_a-zA-Z0-9]*$")
)

func init() {
	prometheus.MustRegister(counterNodesNotFound)
}

const (
	// InternalAppName is a reserved name used for token generation. For
	// backward compatibility and historical purpose, the value remained
	// "tsr" when the name of the daemon changed to "tsurud".
	InternalAppName = "tsr"

	TsuruServicesEnvVar = "TSURU_SERVICES"
	defaultAppDir       = "/home/application/current"

	routerNone = "none"
)

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App struct {
	Env             map[string]bind.EnvVar
	ServiceEnvs     []bind.ServiceEnvVar
	Platform        string `bson:"framework"`
	PlatformVersion string
	Name            string
	CName           []string
	Teams           []string
	TeamOwner       string
	Owner           string
	Plan            appTypes.Plan
	UpdatePlatform  bool
	Lock            appTypes.AppLock
	Pool            string
	Description     string
	Router          string
	RouterOpts      map[string]string
	Deploys         uint
	Tags            []string
	Error           string
	Routers         []appTypes.AppRouter
	Metadata        appTypes.Metadata

	// UUID is a v4 UUID lazily generated on the first call to GetUUID()
	UUID string

	// InterApp Properties implemented by provision.InterAppProvisioner
	// it is lazy generated on the first call to FillInternalAddresses
	InternalAddresses []provision.AppInternalAddress `json:",omitempty" bson:"-"`

	Quota quota.Quota

	ctx         context.Context
	builder     builder.Builder
	provisioner provision.Provisioner
}

var (
	_ provision.App      = &App{}
	_ rebuild.RebuildApp = &App{}
)

func (app *App) ReplaceContext(ctx context.Context) {
	app.ctx = ctx
}

func (app *App) Context() context.Context {
	if app.ctx != nil {
		return app.ctx
	}
	return context.Background()
}

func (app *App) getBuilder() (builder.Builder, error) {
	if app.builder != nil {
		return app.builder, nil
	}
	p, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	app.builder, err = builder.GetForProvisioner(p)
	return app.builder, err
}

func (app *App) FillInternalAddresses() error {
	provisioner, err := app.getProvisioner()
	if err != nil {
		return err
	}

	if interAppProvisioner, ok := provisioner.(provision.InterAppProvisioner); ok {
		app.InternalAddresses, err = interAppProvisioner.InternalAddresses(app.ctx, app)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *App) CleanImage(img string) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	cleanProv, ok := prov.(provision.CleanImageProvisioner)
	if !ok {
		return nil
	}
	return cleanProv.CleanImage(app.Name, img)
}

func (app *App) getProvisioner() (provision.Provisioner, error) {
	var err error
	if app.provisioner == nil {
		app.provisioner, err = pool.GetProvisionerForPool(app.ctx, app.Pool)
	}
	return app.provisioner, err
}

// Units returns the list of units.
func (app *App) Units() ([]provision.Unit, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return []provision.Unit{}, err
	}
	units, err := prov.Units(context.TODO(), app)
	if units == nil {
		// This is unusual but was done because previously this method didn't
		// return an error. This ensures we always return an empty list instead
		// of nil to preserve compatibility with old clients.
		units = []provision.Unit{}
	}
	return units, err
}

// MarshalJSON marshals the app in json format.
func (app *App) MarshalJSON() ([]byte, error) {
	var errMsgs []string
	result := make(map[string]interface{})
	result["name"] = app.Name
	result["platform"] = app.Platform
	if version := app.GetPlatformVersion(); version != "latest" {
		result["platform"] = fmt.Sprintf("%s:%s", app.Platform, version)
	}
	prov, err := app.getProvisioner()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get provisioner name: %+v", err))
	}
	if prov != nil {
		provisionerName := prov.GetName()
		result["provisioner"] = provisionerName
		cluster, clusterErr := servicemanager.Cluster.FindByPool(app.ctx, provisionerName, app.Pool)
		if clusterErr != nil && clusterErr != provisionTypes.ErrNoCluster {
			errMsgs = append(errMsgs, fmt.Sprintf("unable to get cluster name: %+v", clusterErr))
		}
		if cluster != nil {
			result["cluster"] = cluster.Name
		}
	}
	result["teams"] = app.Teams
	units, err := app.Units()
	result["units"] = units
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to list app units: %+v", err))
	}
	plan := map[string]interface{}{
		"name":     app.Plan.Name,
		"memory":   app.Plan.Memory,
		"swap":     app.Plan.Swap,
		"cpushare": app.Plan.CpuShare,
		"cpumilli": app.Plan.CPUMilli,
		"override": app.Plan.Override,
	}
	routers, err := app.GetRoutersWithAddr()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get app addresses: %+v", err))
	}
	if len(routers) > 0 {
		result["ip"] = routers[0].Address
		plan["router"] = routers[0].Name
		result["router"] = routers[0].Name
		result["routeropts"] = routers[0].Opts
	}
	result["cname"] = app.CName
	result["owner"] = app.Owner
	result["pool"] = app.Pool
	result["description"] = app.Description
	result["deploys"] = app.Deploys
	result["teamowner"] = app.TeamOwner
	result["plan"] = plan
	result["lock"] = app.Lock
	result["tags"] = app.Tags
	result["routers"] = routers
	result["metadata"] = app.Metadata
	q, err := app.GetQuota()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get app quota: %+v", err))
	}
	if q != nil {
		result["quota"] = *q
	}
	if len(app.InternalAddresses) > 0 {
		result["internalAddresses"] = app.InternalAddresses
	}
	autoscale, err := app.AutoScaleInfo()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale info: %+v", err))
	}
	if autoscale != nil {
		result["autoscale"] = autoscale
	}
	autoscaleRec, err := app.VerticalAutoScaleRecommendations()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale recommendation info: %+v", err))
	}
	if autoscaleRec != nil {
		result["autoscaleRecommendation"] = autoscaleRec
	}
	unitMetrics, err := app.UnitsMetrics()
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get units metrics: %+v", err))
	}
	if unitMetrics != nil {
		result["unitsMetrics"] = unitMetrics
	}
	volumeBinds, err := servicemanager.Volume.BindsForApp(app.ctx, nil, app.Name)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get volume binds: %+v", err))
	}
	if volumeBinds != nil {
		result["volumeBinds"] = volumeBinds
	}
	sis, err := service.GetServiceInstancesBoundToApp(app.Name)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get service instance bound to app: %+v", err))
	}
	result["serviceInstanceBinds"] = make([]interface{}, 0)
	for _, si := range sis {
		result["serviceInstanceBinds"] = append(result["serviceInstanceBinds"].([]interface{}), map[string]interface{}{
			"service":  si.ServiceName,
			"instance": si.Name,
			"plan":     si.PlanName,
		})
	}
	if len(errMsgs) > 0 {
		result["error"] = strings.Join(errMsgs, "\n")
	}
	return json.Marshal(&result)
}

// GetByName queries the database to find an app identified by the given
// name.
func GetByName(ctx context.Context, name string) (*App, error) {
	var app App
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"name": name}).One(&app)
	if err == mgo.ErrNotFound {
		return nil, appTypes.ErrAppNotFound
	}
	app.ctx = ctx
	return &app, err
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of the following steps:
//
//       1. Save the app in the database
//       2. Provision the app using the provisioner
func CreateApp(ctx context.Context, app *App, user *auth.User) error {
	if app.ctx == nil {
		app.ctx = ctx
	}
	if _, err := GetByName(ctx, app.GetName()); err != appTypes.ErrAppNotFound {
		if err != nil {
			return errors.WithMessage(err, "unable to check if app already exists")
		}
		return &appTypes.AppCreationError{Err: ErrAppAlreadyExists, App: app.GetName()}
	}
	var err error
	err = app.SetPool()
	if err != nil {
		return err
	}
	appPool, err := pool.GetPoolByName(ctx, app.GetPool())
	if err != nil {
		return err
	}
	plan, err := appPool.GetDefaultPlan()
	if err != nil {
		return err
	}
	if app.Plan.Name != "" {
		plan, err = servicemanager.Plan.FindByName(ctx, app.Plan.Name)
	}
	if err != nil {
		return err
	}
	app.Plan = *plan
	err = app.configureCreateRouters()
	if err != nil {
		return err
	}
	app.Teams = []string{app.TeamOwner}
	app.Owner = user.Email
	app.Tags = processTags(app.Tags)
	if app.Platform != "" {
		app.Platform, app.PlatformVersion, err = app.getPlatformNameAndVersion(ctx, app.Platform)
		if err != nil {
			return err
		}
	}
	err = app.validateNew(ctx)
	if err != nil {
		return err
	}
	actions := []*action.Action{
		&reserveUserApp,
		&insertApp,
		&createAppToken,
		&exportEnvironmentsAction,
		&provisionApp,
		&addRouterBackend,
	}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(ctx, app, user)
	if err != nil {
		return &appTypes.AppCreationError{App: app.Name, Err: err}
	}
	return nil
}

func (app *App) configureCreateRouters() error {
	if len(app.Routers) > 0 {
		return nil
	}
	if app.Router == routerNone {
		app.Router = ""
		app.RouterOpts = nil
		return nil
	}
	var err error
	if app.Router == "" {
		var appPool *pool.Pool
		appPool, err = pool.GetPoolByName(app.ctx, app.GetPool())
		if err != nil {
			return err
		}
		app.Router, err = appPool.GetDefaultRouter()
	} else {
		_, err = router.Get(app.ctx, app.Router)
	}
	if err != nil {
		return err
	}
	app.Routers = []appTypes.AppRouter{{
		Name: app.Router,
		Opts: app.RouterOpts,
	}}
	app.Router = ""
	app.RouterOpts = nil
	return nil
}

type UpdateAppArgs struct {
	UpdateData    App
	Writer        io.Writer
	ShouldRestart bool
}

// Update changes informations of the application.
func (app *App) Update(args UpdateAppArgs) (err error) {
	description := args.UpdateData.Description
	poolName := args.UpdateData.Pool
	teamOwner := args.UpdateData.TeamOwner
	platform := args.UpdateData.Platform
	tags := processTags(args.UpdateData.Tags)
	oldApp := *app

	if description != "" {
		app.Description = description
	}
	if poolName != "" {
		app.Pool = poolName
		app.provisioner = nil
		_, err = app.getPoolForApp(app.Pool)
		if err != nil {
			return err
		}
	}
	newProv, err := app.getProvisioner()
	if err != nil {
		return err
	}
	oldProv, err := oldApp.getProvisioner()
	if err != nil {
		return err
	}
	if args.UpdateData.Plan.Name != "" {
		plan, errFind := servicemanager.Plan.FindByName(app.ctx, args.UpdateData.Plan.Name)
		if errFind != nil {
			return errFind
		}
		app.Plan = *plan
	}
	app.Plan.MergeOverride(args.UpdateData.Plan.Override)
	if teamOwner != "" {
		team, errTeam := servicemanager.Team.FindByName(app.ctx, teamOwner)
		if errTeam != nil {
			return errTeam
		}
		app.TeamOwner = team.Name
		defer func() {
			if err == nil {
				app.Grant(team)
			}
		}()
	}
	if tags != nil {
		app.Tags = tags
	}
	err = args.UpdateData.Metadata.Validate()
	if err != nil {
		return err
	}
	app.Metadata.Update(args.UpdateData.Metadata)
	if platform != "" {
		var p, v string
		p, v, err = app.getPlatformNameAndVersion(app.ctx, platform)
		if err != nil {
			return err
		}
		if app.Platform != p || app.PlatformVersion != v {
			app.UpdatePlatform = true
		}
		app.Platform = p
		app.PlatformVersion = v
	}
	if args.UpdateData.UpdatePlatform {
		app.UpdatePlatform = true
	}
	err = app.validate()
	if err != nil {
		return err
	}
	actions := []*action.Action{
		&saveApp,
	}
	updatePipelineAdded := false
	if newProv.GetName() == oldProv.GetName() {
		updatePipelineAdded = true
		actions = append(actions, &updateAppProvisioner)
	}
	if newProv.GetName() != oldProv.GetName() {
		defer func() {
			rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, args.Writer)
		}()
		err = validateVolumes(app.ctx, app)
		if err != nil {
			return err
		}
		actions = append(actions,
			&provisionAppNewProvisioner,
			&provisionAppAddUnits,
			&destroyAppOldProvisioner)
	} else if !reflect.DeepEqual(app.Plan, oldApp.Plan) && args.ShouldRestart {
		actions = append(actions, &restartApp)
	} else if app.Pool != oldApp.Pool && !updatePipelineAdded {
		actions = append(actions, &restartApp)
	}
	return action.NewPipeline(actions...).Execute(app.ctx, app, &oldApp, args.Writer)
}

func validateVolumes(ctx context.Context, app *App) error {
	volumes, err := servicemanager.Volume.ListByApp(ctx, app.Name)
	if err != nil {
		return err
	}
	if len(volumes) > 0 {
		return fmt.Errorf("can't change the provisioner of an app with binded volumes")
	}
	return nil
}

func (app *App) getPlatformNameAndVersion(ctx context.Context, platform string) (string, string, error) {
	repo, version := image.SplitImageName(platform)
	p, err := servicemanager.Platform.FindByName(ctx, repo)
	if err != nil {
		return "", "", err
	}
	reg, err := app.GetRegistry()
	if err != nil {
		return "", "", err
	}

	if version != "latest" {
		_, err = servicemanager.PlatformImage.FindImage(ctx, reg, p.Name, version)
		if err != nil {
			return p.Name, "", err
		}
	}

	return p.Name, version, nil
}

func processTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	processedTags := []string{}
	usedTags := make(map[string]bool)
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 0 && !usedTags[tag] {
			processedTags = append(processedTags, tag)
			usedTags[tag] = true
		}
	}
	return processedTags
}

// unbind takes all service instances that are bound to the app, and unbind
// them. This method is used by Destroy (before destroying the app, it unbinds
// all service instances). Refer to Destroy docs for more details.
func (app *App) unbind(evt *event.Event, requestID string) error {
	instances, err := service.GetServiceInstancesBoundToApp(app.Name)
	if err != nil {
		return err
	}
	var msg string
	var addMsg = func(instanceName string, reason error) {
		if msg == "" {
			msg = "Failed to unbind the following instances:\n"
		}
		msg += fmt.Sprintf("- %s (%s)", instanceName, reason.Error())
	}
	for _, instance := range instances {
		err = instance.UnbindApp(service.UnbindAppArgs{
			App:         app,
			Restart:     false,
			ForceRemove: true,
			Event:       evt,
			RequestID:   requestID,
		})
		if err != nil {
			addMsg(instance.Name, err)
		}
	}
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (app *App) unbindVolumes() error {
	volumes, err := servicemanager.Volume.ListByApp(app.ctx, app.Name)
	if err != nil {
		return errors.Wrap(err, "Unable to list volumes for unbind")
	}
	for _, v := range volumes {
		var binds []volumeTypes.VolumeBind
		binds, err = servicemanager.Volume.Binds(app.ctx, &v)
		if err != nil {
			return errors.Wrap(err, "Unable to list volume binds for unbind")
		}
		for _, b := range binds {
			err = servicemanager.Volume.UnbindApp(app.ctx, &volumeTypes.BindOpts{
				Volume:     &v,
				AppName:    app.Name,
				MountPoint: b.ID.MountPoint,
			})
			if err != nil {
				return errors.Wrapf(err, "Unable to unbind volume %q in %q", app.Name, b.ID.MountPoint)
			}
		}
	}
	return nil
}

// Delete deletes an app.
func Delete(ctx context.Context, app *App, evt *event.Event, requestID string) error {
	w := evt
	isSwapped, swappedWith, err := router.IsSwapped(app.GetName())
	if err != nil {
		return errors.Wrap(err, "unable to check if app is swapped")
	}
	if isSwapped {
		return errors.Errorf("application is swapped with %q, cannot remove it", swappedWith)
	}
	appName := app.Name
	fmt.Fprintf(w, "---- Removing application %q...\n", appName)
	var hasErrors bool
	defer func() {
		var problems string
		if hasErrors {
			problems = " Some errors occurred during removal."
		}
		fmt.Fprintf(w, "---- Done removing application.%s\n", problems)
	}()
	logErr := func(msg string, err error) {
		msg = fmt.Sprintf("%s: %s", msg, err)
		fmt.Fprintf(w, "%s\n", msg)
		log.Errorf("[delete-app: %s] %s", appName, msg)
		hasErrors = true
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	err = registry.RemoveAppImages(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove images from registry for app %s: %s", appName, err)
	}
	if cleanProv, ok := prov.(provision.CleanImageProvisioner); ok {
		var versions appTypes.AppVersions
		versions, err = servicemanager.AppVersion.AppVersions(ctx, app)
		if err != nil {
			log.Errorf("failed to list versions for app %s: %s", appName, err)
		}
		for _, version := range versions.Versions {
			var imgs []string
			if version.BuildImage != "" {
				imgs = append(imgs, version.BuildImage)
			}
			if version.DeployImage != "" {
				imgs = append(imgs, version.DeployImage)
			}
			for _, img := range imgs {
				err = cleanProv.CleanImage(appName, img)
				if err != nil {
					log.Errorf("failed to remove image %q from provisioner %s: %s", img, appName, err)
				}
			}
		}
	}
	err = servicemanager.AppVersion.DeleteVersions(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove image names from storage for app %s: %s", appName, err)
	}
	err = app.unbind(evt, requestID)
	if err != nil {
		logErr("Unable to unbind app", err)
	}
	routers := app.GetRouters()
	for _, appRouter := range routers {
		var r router.Router
		r, err = router.Get(ctx, appRouter.Name)
		if err == nil {
			err = r.RemoveBackend(ctx, app)
		}
		if err != nil {
			logErr("Failed to remove router backend", err)
		}
	}
	err = router.Remove(app.Name)
	if err != nil {
		logErr("Failed to remove router backend from database", err)
	}
	err = app.unbindVolumes()
	if err != nil {
		logErr("Unable to unbind volumes", err)
	}
	token := app.Env["TSURU_APP_TOKEN"].Value
	err = AuthScheme.AppLogout(ctx, token)
	if err != nil {
		logErr("Unable to remove app token in destroy", err)
	}
	owner, err := auth.GetUserByEmail(app.Owner)
	if err == nil {
		err = servicemanager.UserQuota.Inc(ctx, owner, -1)
	}
	if err != nil {
		logErr("Unable to release app quota", err)
	}

	if plog, ok := servicemanager.AppLog.(appTypes.AppLogServiceProvision); ok {
		err = plog.CleanUp(app.Name)
		if err != nil {
			logErr("Unable to remove logs", err)
		}
	}

	conn, err := db.Conn()
	if err == nil {
		defer conn.Close()
		err = conn.Apps().Remove(bson.M{"name": appName})
	}
	if err != nil {
		logErr("Unable to remove app from db", err)
	}
	// NOTE: some provisioners hold apps' info on their own (e.g. apps.tsuru.io
	// CustomResource on Kubernetes). Deleting the app on provisioner as the last
	// step of removal, we may give time enough to external components
	// (e.g. tsuru/kubernetes-router) that depend on the provisioner's app info
	// finish as expected.
	err = prov.Destroy(ctx, app)
	if err != nil {
		logErr("Unable to destroy app in provisioner", err)
	}
	return nil
}

// DeleteVersion deletes an app version.
func (app *App) DeleteVersion(ctx context.Context, w io.Writer, versionStr string) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("\n ---> Deleting version %s of app %s", versionStr, app.Name)
	fmt.Fprintf(w, "%s\n", msg)
	var hasErrors bool
	defer func() {
		var problems string
		if hasErrors {
			problems = " Some errors occurred during removal."
		}
		fmt.Fprintf(w, "---- Done removing application version %s.%s\n", versionStr, problems)
	}()

	logErr := func(msg string, err error) {
		msg = fmt.Sprintf("%s: %s", msg, err)
		fmt.Fprintf(w, "%s\n", msg)
		log.Errorf("[delete-app-version: %s-%s] %s", app.Name, versionStr, msg)
		hasErrors = true
	}

	_, version, err := app.explicitVersion(versionStr)
	if err != nil {
		return err
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}

	err = prov.DestroyVersion(ctx, app, version)
	if err != nil {
		logErr("Unable to destroy app in provisioner", err)
	}

	return nil
}

func (app *App) BindUnit(unit *provision.Unit) error {
	instances, err := service.GetServiceInstancesBoundToApp(app.Name)
	if err != nil {
		return err
	}
	var i int
	var instance service.ServiceInstance
	for i, instance = range instances {
		err = instance.BindUnit(app, unit)
		if err != nil {
			log.Errorf("Error binding the unit %s with the service instance %s: %s", unit.ID, instance.Name, err)
			break
		}
	}
	if err != nil {
		for j := i - 1; j >= 0; j-- {
			instance = instances[j]
			rollbackErr := instance.UnbindUnit(app, unit)
			if rollbackErr != nil {
				log.Errorf("Error unbinding unit %s with the service instance %s during rollback: %s", unit.ID, instance.Name, rollbackErr)
			}
		}
	}
	return err
}

func (app *App) UnbindUnit(unit *provision.Unit) error {
	instances, err := service.GetServiceInstancesBoundToApp(app.Name)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		err = instance.UnbindUnit(app, unit)
		if err != nil {
			log.Errorf("Error unbinding the unit %s with the service instance %s: %s", unit.ID, instance.Name, err)
		}
	}
	return nil
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (app *App) AddUnits(n uint, process, versionStr string, w io.Writer) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
	}

	err := app.ensureNoAutoscaler(process)
	if err != nil {
		return err
	}

	units, err := app.Units()
	if err != nil {
		return err
	}
	for _, u := range units {
		if (u.Status == provision.StatusAsleep) || (u.Status == provision.StatusStopped) {
			return errors.New("Cannot add units to an app that has stopped or sleeping units")
		}
	}
	version, err := app.getVersion(app.ctx, versionStr)
	if err != nil {
		return err
	}
	w = app.withLogWriter(w)
	err = action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
	).Execute(app.ctx, app, n, w, process, version)
	rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, w)
	if err != nil {
		return newErrorWithLog(err, app, "add units")
	}
	return nil
}

func (app *App) ensureNoAutoscaler(process string) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if ok {
		autoscales, err := autoscaleProv.GetAutoScale(app.ctx, app)
		if err != nil {
			return err
		}

		for _, as := range autoscales {
			if as.Process == process {
				return errors.New("cannot add units to an app with autoscaler configured, please update autoscale settings")
			}
		}
	}
	return nil
}

// RemoveUnits removes n units from the app. It's a process composed of
// multiple steps:
//
//     1. Remove units from the provisioner
//     2. Update quota
func (app *App) RemoveUnits(ctx context.Context, n uint, process, versionStr string, w io.Writer) error {
	err := app.ensureNoAutoscaler(process)
	if err != nil {
		return err
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	w = app.withLogWriter(w)
	version, err := app.getVersion(ctx, versionStr)
	if err != nil {
		return err
	}
	err = prov.RemoveUnits(ctx, app, n, process, version, w)
	rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, w)
	if err != nil {
		return newErrorWithLog(err, app, "remove units")
	}
	return nil
}

// SetUnitStatus changes the status of the given unit.
func (app *App) SetUnitStatus(unitName string, status provision.Status) error {
	units, err := app.Units()
	if err != nil {
		return err
	}
	for _, unit := range units {
		if strings.HasPrefix(unit.ID, unitName) {
			prov, err := app.getProvisioner()
			if err != nil {
				return err
			}
			unitProv, ok := prov.(provision.UnitStatusProvisioner)
			if !ok {
				return nil
			}
			return unitProv.SetUnitStatus(unit, status)
		}
	}
	return &provision.UnitNotFoundError{ID: unitName}
}

type UpdateUnitsResult struct {
	ID    string
	Found bool
}

func findNodeForNodeData(ctx context.Context, nodeData provision.NodeStatusData) (provision.Node, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	provErrors := tsuruErrors.NewMultiError()
	for _, p := range provisioners {
		if nodeProv, ok := p.(provision.NodeProvisioner); ok {
			var node provision.Node
			if len(nodeData.Addrs) == 1 {
				node, err = nodeProv.GetNode(ctx, nodeData.Addrs[0])
			} else {
				node, err = nodeProv.NodeForNodeData(ctx, nodeData)
			}
			if err == nil {
				return node, nil
			}
			if errors.Cause(err) != provision.ErrNodeNotFound {
				provErrors.Add(err)
			}
		}
	}
	if err = provErrors.ToError(); err != nil {
		return nil, err
	}
	return nil, provision.ErrNodeNotFound
}

// UpdateNodeStatus updates the status of the given node and its units,
// returning a map which units were found during the update.
func UpdateNodeStatus(ctx context.Context, nodeData provision.NodeStatusData) ([]UpdateUnitsResult, error) {
	node, findNodeErr := findNodeForNodeData(ctx, nodeData)
	var nodeAddresses []string
	if findNodeErr == nil {
		nodeAddresses = []string{node.Address()}
	} else {
		nodeAddresses = nodeData.Addrs
	}
	if healer.HealerInstance != nil {
		err := healer.HealerInstance.UpdateNodeData(nodeAddresses, nodeData.Checks)
		if err != nil {
			log.Errorf("[update node status] unable to set node status in healer: %s", err)
		}
	}
	if findNodeErr == provision.ErrNodeNotFound {
		counterNodesNotFound.Inc()
		log.Errorf("[update node status] node not found with nodedata: %#v", nodeData)
		result := make([]UpdateUnitsResult, len(nodeData.Units))
		for i, unitData := range nodeData.Units {
			result[i] = UpdateUnitsResult{ID: unitData.ID, Found: false}
		}
		return result, nil
	}
	if findNodeErr != nil {
		return nil, findNodeErr
	}
	unitProv, ok := node.Provisioner().(provision.UnitStatusProvisioner)
	if !ok {
		return []UpdateUnitsResult{}, nil
	}
	result := make([]UpdateUnitsResult, len(nodeData.Units))
	for i, unitData := range nodeData.Units {
		unit := provision.Unit{ID: unitData.ID, Name: unitData.Name}
		err := unitProv.SetUnitStatus(unit, unitData.Status)
		_, isNotFound := err.(*provision.UnitNotFoundError)
		if err != nil && !isNotFound {
			return nil, err
		}
		result[i] = UpdateUnitsResult{ID: unitData.ID, Found: !isNotFound}
	}
	return result, nil
}

// available returns true if at least one of N units is started or unreachable.
func (app *App) available() bool {
	units, err := app.Units()
	if err != nil {
		return false
	}
	for _, unit := range units {
		if unit.Available() {
			return true
		}
	}
	return false
}

func (app *App) findTeam(team *authTypes.Team) (int, bool) {
	for i, teamName := range app.Teams {
		if teamName == team.Name {
			return i, true
		}
	}
	return -1, false
}

// Grant allows a team to have access to an app. It returns an error if the
// team already have access to the app.
func (app *App) Grant(team *authTypes.Team) error {
	if _, found := app.findTeam(team); found {
		return ErrAlreadyHaveAccess
	}
	app.Teams = append(app.Teams, team.Name)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$addToSet": bson.M{"teams": team.Name}})
	if err != nil {
		return err
	}

	if err != nil {
		conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$pull": bson.M{"teams": team.Name}})
		return err
	}

	return nil
}

// Revoke removes the access from a team. It returns an error if the team do
// not have access to the app.
func (app *App) Revoke(team *authTypes.Team) error {
	if len(app.Teams) == 1 {
		return ErrCannotOrphanApp
	}
	index, found := app.findTeam(team)
	if !found {
		return ErrNoAccess
	}
	last := len(app.Teams) - 1
	app.Teams[index] = app.Teams[last]
	app.Teams = app.Teams[:last]
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$pull": bson.M{"teams": team.Name}})
	if err != nil {
		return err
	}
	return nil
}

// GetTeams returns a slice of teams that have access to the app.
func (app *App) GetTeams() []authTypes.Team {
	t, _ := servicemanager.Team.FindByNames(app.ctx, app.Teams)
	return t
}

func (app *App) SetPool() error {
	poolName, err := app.getPoolForApp(app.Pool)
	if err != nil {
		return err
	}
	if poolName == "" {
		var p *pool.Pool
		p, err = pool.GetDefaultPool(app.ctx)
		if err != nil {
			return err
		}
		poolName = p.Name
	}
	app.Pool = poolName
	p, err := pool.GetPoolByName(app.ctx, poolName)
	if err != nil {
		return err
	}
	return app.validateTeamOwner(p)
}

func (app *App) getPoolForApp(poolName string) (string, error) {
	if poolName == "" {
		pools, err := pool.ListPoolsForTeam(app.ctx, app.TeamOwner)
		if err != nil {
			return "", err
		}
		if len(pools) > 1 {
			publicPools, err := pool.ListPublicPools(app.ctx)
			if err != nil {
				return "", err
			}
			var names []string
			for _, p := range append(pools, publicPools...) {
				names = append(names, fmt.Sprintf("%q", p.Name))
			}
			return "", errors.Errorf("you have access to %s pools. Please choose one in app creation", strings.Join(names, ","))
		}
		if len(pools) == 0 {
			return "", nil
		}
		return pools[0].Name, nil
	}
	pool, err := pool.GetPoolByName(app.ctx, poolName)
	if err != nil {
		return "", err
	}
	return pool.Name, nil
}

// setEnv sets the given environment variable in the app.
func (app *App) setEnv(env bind.EnvVar) {
	if app.Env == nil {
		app.Env = make(map[string]bind.EnvVar)
	}
	app.Env[env.Name] = env
	if env.Public {
		servicemanager.AppLog.Add(app.Name, fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru", "api")
	}
}

// getEnv returns the environment variable if it's declared in the app. It will
// return an error if the variable is not defined in this app.
func (app *App) getEnv(name string) (bind.EnvVar, error) {
	if env, ok := app.Env[name]; ok {
		return env, nil
	}
	return bind.EnvVar{}, errors.New("Environment variable not declared for this app.")
}

// validateNew checks app name format, pool and plan
func (app *App) validateNew(ctx context.Context) error {
	if app.Name == InternalAppName || !validation.ValidateName(app.Name) {
		msg := "Invalid app name, your app should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return app.validate()
}

// validate checks app pool and plan
func (app *App) validate() error {
	err := app.validatePool()
	if err != nil {
		return err
	}
	return app.validatePlan()
}

func (app *App) validatePlan() error {
	pool, err := pool.GetPoolByName(app.ctx, app.Pool)
	if err != nil {
		return err
	}
	plans, err := pool.GetPlans()
	if err != nil {
		return err
	}
	planSet := set.FromSlice(plans)
	if !planSet.Includes(app.Plan.Name) {
		msg := fmt.Sprintf("App plan %q is not allowed on pool %q", app.Plan.Name, pool.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func (app *App) validatePool() error {
	pool, err := pool.GetPoolByName(app.ctx, app.Pool)
	if err != nil {
		return err
	}
	err = app.validateTeamOwner(pool)
	if err != nil {
		return err
	}
	instances, err := service.GetServiceInstancesBoundToApp(app.Name)
	if err != nil {
		return err
	}
	if len(instances) > 0 {
		serviceNames := make([]string, len(instances))
		for i, instance := range instances {
			serviceNames[i] = instance.ServiceName
		}
		err = app.ValidateService(serviceNames...)
		if err != nil {
			return err
		}
	}

	return pool.ValidateRouters(app.GetRouters())
}

func (app *App) validateTeamOwner(p *pool.Pool) error {
	_, err := servicemanager.Team.FindByName(app.ctx, app.TeamOwner)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	poolTeams, err := p.GetTeams()
	if err != nil && err != pool.ErrPoolHasNoTeam {
		msg := fmt.Sprintf("failed to get pool %q teams", p.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	for _, team := range poolTeams {
		if team == app.TeamOwner {
			return nil
		}
	}
	msg := fmt.Sprintf("App team owner %q has no access to pool %q", app.TeamOwner, p.Name)
	return &tsuruErrors.ValidationError{Message: msg}
}

func (app *App) ValidateService(services ...string) error {
	pool, err := pool.GetPoolByName(app.ctx, app.Pool)
	if err != nil {
		return err
	}

	poolServices, err := servicemanager.Pool.Services(app.ctx, app.Pool)
	if err != nil {
		return err
	}
	for _, svc := range services {
		valid := false
		for _, v := range poolServices {
			if v == svc {
				valid = true
				break
			}
		}
		if !valid {
			msg := fmt.Sprintf("service %q is not available for pool %q.", svc, pool.Name)

			if len(poolServices) > 0 {
				msg += fmt.Sprintf(" Available services are: %q", strings.Join(poolServices, ", "))
			}
			return &tsuruErrors.ValidationError{Message: msg}
		}
	}
	return nil
}

// InstanceEnvs returns a map of environment variables that belongs to the
// given service and service instance.
func (app *App) InstanceEnvs(serviceName, instanceName string) map[string]bind.EnvVar {
	envs := make(map[string]bind.EnvVar)
	for _, env := range app.ServiceEnvs {
		if env.ServiceName == serviceName && env.InstanceName == instanceName {
			envs[env.Name] = env.EnvVar
		}
	}
	return envs
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (app *App) Run(cmd string, w io.Writer, args provision.RunArgs) error {
	if !args.Isolated && !app.available() {
		return errors.New("App must be available to run non-isolated commands")
	}
	logWriter := LogWriter{AppName: app.Name, Source: "app-run"}
	logWriter.Async()
	defer logWriter.Close()
	logWriter.Write([]byte(fmt.Sprintf("running '%s'", cmd)))
	return app.run(cmd, io.MultiWriter(w, &logWriter), args)
}

func cmdsForExec(cmd string) []string {
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := fmt.Sprintf("[ -d %s ] && cd %s", defaultAppDir, defaultAppDir)
	return []string{"/bin/sh", "-c", fmt.Sprintf("%s; %s; %s", source, cd, cmd)}
}

func (app *App) run(cmd string, w io.Writer, args provision.RunArgs) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	execProv, ok := prov.(provision.ExecutableProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "running commands"}
	}
	opts := provision.ExecOptions{
		App:    app,
		Stdout: w,
		Stderr: w,
		Cmds:   cmdsForExec(cmd),
	}
	units, err := app.Units()
	if err != nil {
		return err
	}
	if args.Once && len(units) > 0 {
		opts.Units = []string{units[0].ID}
	} else if !args.Isolated {
		for _, u := range units {
			opts.Units = append(opts.Units, u.ID)
		}
	}
	return execProv.ExecuteCommand(app.ctx, opts)
}

// Restart runs the restart hook for the app, writing its output to w.
func (app *App) Restart(ctx context.Context, process, versionStr string, w io.Writer) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("---- Restarting process %q ----", process)
	if process == "" {
		msg = fmt.Sprintf("---- Restarting the app %q ----", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(versionStr)
	if err != nil {
		return err
	}
	err = prov.Restart(ctx, app, process, version, w)
	if err != nil {
		log.Errorf("[restart] error on restart the app %s - %s", app.Name, err)
		return newErrorWithLog(err, app, "restart")
	}
	rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, w)
	return nil
}

// vpPair represents each version-process pair
type vpPair struct {
	version int
	process string
}

func cleanupOtherProcesses(vpMap map[vpPair]int, process string) {
	for pair := range vpMap {
		if pair.process != process {
			delete(vpMap, pair)
		}
	}
}

func generateVersionProcessPastUnitsMap(version appTypes.AppVersion, units []provision.Unit, process string) map[vpPair]int {
	pastUnitsMap := map[vpPair]int{}
	if version == nil {
		for _, unit := range units {
			vp := vpPair{
				version: unit.Version,
				process: unit.ProcessName,
			}
			if _, ok := pastUnitsMap[vp]; !ok {
				pastUnitsMap[vp] = 1
			} else {
				pastUnitsMap[vp]++
			}
		}
	} else {
		for _, unit := range units {
			if unit.Version != version.Version() {
				continue
			}
			vp := vpPair{
				version: unit.Version,
				process: unit.ProcessName,
			}
			if _, ok := pastUnitsMap[vp]; !ok {
				pastUnitsMap[vp] = 1
			} else {
				pastUnitsMap[vp]++
			}
		}
	}

	if process != "" {
		cleanupOtherProcesses(pastUnitsMap, process)
	}

	return pastUnitsMap
}

func (app *App) updatePastUnits(ctx context.Context, version appTypes.AppVersion, process string) error {
	units, err := app.provisioner.Units(ctx, app)
	if err != nil {
		return err
	}

	vpMap := generateVersionProcessPastUnitsMap(version, units, process)

	for vp, replicas := range vpMap {
		versionStr := strconv.Itoa(vp.version)
		v, err := app.getVersion(ctx, versionStr)
		if err != nil {
			return err
		}
		err = v.UpdatePastUnits(vp.process, replicas)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *App) Stop(ctx context.Context, w io.Writer, process, versionStr string) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("\n ---> Stopping the process %q", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Stopping the app %q", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(versionStr)
	if err != nil {
		return err
	}

	err = app.updatePastUnits(ctx, version, process)
	if err != nil {
		return err
	}

	err = prov.Stop(ctx, app, process, version, w)
	if err != nil {
		log.Errorf("[stop] error on stop the app %s - %s", app.Name, err)
		return err
	}
	return nil
}

func (app *App) Sleep(ctx context.Context, w io.Writer, process, versionStr string, proxyURL *url.URL) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	sleepProv, ok := prov.(provision.SleepableProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "sleeping"}
	}
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("\n ---> Putting the process %q to sleep", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Putting the app %q to sleep", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	routers := app.GetRouters()
	for _, appRouter := range routers {
		var r router.Router
		r, err = router.Get(ctx, appRouter.Name)
		if err != nil {
			log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
			return err
		}
		if _, isRouterV2 := r.(router.RouterV2); isRouterV2 {
			log.Errorf("Router %s does not support to put app in sleep mode", appRouter.Name)
			return fmt.Errorf("Router %s does not support to put app in sleep mode", appRouter.Name)
		}
		var oldRoutes []*url.URL
		oldRoutes, err = r.Routes(app.ctx, app)
		if err != nil {
			log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
			return err
		}
		err = r.RemoveRoutes(app.ctx, app, oldRoutes)
		if err != nil {
			log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
			return err
		}
		err = r.AddRoutes(app.ctx, app, []*url.URL{proxyURL})
		if err != nil {
			log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
			return err
		}
	}
	version, err := app.getVersionAllowNil(versionStr)
	if err != nil {
		return err
	}
	err = sleepProv.Sleep(ctx, app, process, version)
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		log.Errorf("[sleep] rolling back the sleep %s", app.Name)
		rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, w)
		return newErrorWithLog(err, app, "sleep")
	}
	return nil
}

// GetUnits returns the internal list of units converted to bind.Unit.
func (app *App) GetUnits() ([]bind.Unit, error) {
	provUnits, err := app.Units()
	if err != nil {
		return nil, err
	}
	units := make([]bind.Unit, len(provUnits))
	for i := range provUnits {
		units[i] = &provUnits[i]
	}
	return units, nil
}

// GetName returns the name of the app.
func (app *App) GetName() string {
	return app.Name
}

// GetUUID returns the app v4 UUID. An UUID will be generated
// if it does not exist.
func (app *App) GetUUID() (string, error) {
	if app.UUID != "" {
		return app.UUID, nil
	}
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return "", errors.WithMessage(err, "failed to generate uuid v4")
	}
	conn, err := db.Conn()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"uuid": uuidV4.String()}})
	if err != nil {
		return "", err
	}
	app.UUID = uuidV4.String()
	return app.UUID, nil
}

// GetPool returns the pool of the app.
func (app *App) GetPool() string {
	return app.Pool
}

// GetTeamOwner returns the team owner of the app.
func (app *App) GetTeamOwner() string {
	return app.TeamOwner
}

// GetTeamsName returns the names of the app teams.
func (app *App) GetTeamsName() []string {
	return app.Teams
}

// GetMemory returns the memory limit (in bytes) for the app.
func (app *App) GetMemory() int64 {
	if app.Plan.Override.Memory != nil {
		return *app.Plan.Override.Memory
	}
	return app.Plan.Memory
}

func (app *App) GetMilliCPU() int {
	if app.Plan.Override.CPUMilli != nil {
		return *app.Plan.Override.CPUMilli
	}
	return app.Plan.CPUMilli
}

// GetSwap returns the swap limit (in bytes) for the app.
func (app *App) GetSwap() int64 {
	return app.Plan.Swap
}

// GetCpuShare returns the cpu share for the app.
func (app *App) GetCpuShare() int {
	return app.Plan.CpuShare
}

func (app *App) GetAddresses() ([]string, error) {
	routers, err := app.GetRoutersWithAddr()
	if err != nil {
		return nil, err
	}
	addresses := make([]string, len(routers))
	for i := range routers {
		addresses[i] = routers[i].Address
	}
	return addresses, nil
}

func (app *App) GetInternalBindableAddresses() ([]string, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	interAppProv, ok := prov.(provision.InterAppProvisioner)
	if !ok {
		return nil, nil
	}
	addrs, err := interAppProv.InternalAddresses(app.ctx, app)
	if err != nil {
		return nil, err
	}
	var addresses []string
	for _, addr := range addrs {
		// version addresses are so volatile, they change after every deploy, we don't use them to bind process
		if addr.Version != "" {
			continue
		}
		addresses = append(addresses, fmt.Sprintf("%s://%s:%d", strings.ToLower(addr.Protocol), addr.Domain, addr.Port))
	}
	return addresses, nil
}

func (app *App) GetQuotaInUse() (int, error) {
	units, err := app.Units()
	if err != nil {
		return 0, err
	}
	counter := 0
	for _, u := range units {
		switch u.Status {
		case provision.StatusStarting, provision.StatusStarted, provision.StatusStopped, provision.StatusAsleep:
			counter++
		}
	}
	return counter, nil
}

func (app *App) GetQuota() (*quota.Quota, error) {
	return servicemanager.AppQuota.Get(app.ctx, app)
}

func (app *App) SetQuotaLimit(limit int) error {
	return servicemanager.AppQuota.SetLimit(app.ctx, app, limit)
}

// GetCname returns the cnames of the app.
func (app *App) GetCname() []string {
	return app.CName
}

// GetPlatform returns the platform of the app.
func (app *App) GetPlatform() string {
	return app.Platform
}

// GetPlatformVersion returns the platform version of the app.
func (app *App) GetPlatformVersion() string {
	if app.PlatformVersion == "" {
		return "latest"
	}
	return app.PlatformVersion
}

// GetDeploys returns the amount of deploys of an app.
func (app *App) GetDeploys() uint {
	return app.Deploys
}

func interpolate(mergedEnvs map[string]bind.EnvVar, toInterpolate map[string]string, envName, varName string) {
	delete(toInterpolate, envName)
	if toInterpolate[varName] != "" {
		interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[varName])
		return
	}
	if _, isSet := mergedEnvs[varName]; !isSet {
		return
	}
	env := mergedEnvs[envName]
	env.Value = mergedEnvs[varName].Value
	mergedEnvs[envName] = env
}

// Envs returns a map representing the apps environment variables.
func (app *App) Envs() map[string]bind.EnvVar {
	mergedEnvs := make(map[string]bind.EnvVar, len(app.Env)+len(app.ServiceEnvs)+1)
	toInterpolate := make(map[string]string)
	var toInterpolateKeys []string
	for _, e := range app.Env {
		mergedEnvs[e.Name] = e
		if e.Alias != "" {
			toInterpolate[e.Name] = e.Alias
			toInterpolateKeys = append(toInterpolateKeys, e.Name)
		}
	}
	for _, e := range app.ServiceEnvs {
		mergedEnvs[e.Name] = e.EnvVar
	}
	sort.Strings(toInterpolateKeys)
	for _, envName := range toInterpolateKeys {
		interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}
	mergedEnvs[TsuruServicesEnvVar] = serviceEnvsFromEnvVars(app.ServiceEnvs)
	return mergedEnvs
}

func (app *App) restartIfUnits(w io.Writer) error {
	units, err := app.GetUnits()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(app.ctx, app)
	if err != nil {
		return err
	}
	err = prov.Restart(app.ctx, app, "", version, w)
	if err != nil {
		return newErrorWithLog(err, app, "restart")
	}
	return nil
}

// AddCName adds a CName to app. It updates the attribute,
// calls the SetCName function on the provisioner and saves
// the app in the database, returning an error when it cannot save the change
// in the database or add the CName on the provisioner.
func (app *App) AddCName(cnames ...string) error {
	actions := []*action.Action{
		&validateNewCNames,
		&setNewCNamesToProvisioner,
		&saveCNames,
		&updateApp,
	}
	err := action.NewPipeline(actions...).Execute(app.ctx, app, cnames)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	return err
}

func (app *App) RemoveCName(cnames ...string) error {
	actions := []*action.Action{
		&checkCNameExists,
		&unsetCNameFromProvisioner,
		&removeCNameFromDatabase,
		&removeCNameFromApp,
	}
	err := action.NewPipeline(actions...).Execute(app.ctx, app, cnames)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	return err
}

func serviceEnvsFromEnvVars(vars []bind.ServiceEnvVar) bind.EnvVar {
	type serviceInstanceEnvs struct {
		InstanceName string            `json:"instance_name"`
		Envs         map[string]string `json:"envs"`
	}
	result := map[string][]serviceInstanceEnvs{}
	for _, v := range vars {
		found := false
		for i, instanceList := range result[v.ServiceName] {
			if instanceList.InstanceName == v.InstanceName {
				result[v.ServiceName][i].Envs[v.Name] = v.Value
				found = true
				break
			}
		}
		if !found {
			result[v.ServiceName] = append(result[v.ServiceName], serviceInstanceEnvs{
				InstanceName: v.InstanceName,
				Envs:         map[string]string{v.Name: v.Value},
			})
		}
	}
	jsonVal, _ := json.Marshal(result)
	return bind.EnvVar{
		Name:   TsuruServicesEnvVar,
		Value:  string(jsonVal),
		Public: false,
	}
}

func (app *App) AddInstance(addArgs bind.AddInstanceArgs) error {
	if len(addArgs.Envs) == 0 {
		return nil
	}
	if addArgs.Writer != nil {
		fmt.Fprintf(addArgs.Writer, "---- Setting %d new environment variables ----\n", len(addArgs.Envs)+1)
	}
	app.ServiceEnvs = append(app.ServiceEnvs, addArgs.Envs...)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"serviceenvs": app.ServiceEnvs}})
	if err != nil {
		return err
	}
	if addArgs.ShouldRestart {
		return app.restartIfUnits(addArgs.Writer)
	}
	return nil
}

func (app *App) RemoveInstance(removeArgs bind.RemoveInstanceArgs) error {
	lenBefore := len(app.ServiceEnvs)
	for i := 0; i < len(app.ServiceEnvs); i++ {
		se := app.ServiceEnvs[i]
		if se.ServiceName == removeArgs.ServiceName && se.InstanceName == removeArgs.InstanceName {
			app.ServiceEnvs = append(app.ServiceEnvs[:i], app.ServiceEnvs[i+1:]...)
			i--
		}
	}
	toUnset := lenBefore - len(app.ServiceEnvs)
	if toUnset <= 0 {
		return nil
	}
	if removeArgs.Writer != nil {
		fmt.Fprintf(removeArgs.Writer, "---- Unsetting %d environment variables ----\n", toUnset)
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"serviceenvs": app.ServiceEnvs}})
	if err != nil {
		return err
	}
	if removeArgs.ShouldRestart {
		return app.restartIfUnits(removeArgs.Writer)
	}
	return nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// fields in the log instance received as an example.
func (app *App) LastLogs(ctx context.Context, logService appTypes.AppLogService, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	logsProvisioner, ok := prov.(provision.OptionalLogsProvisioner)
	if ok {
		var enabled bool
		var doc string
		enabled, doc, err = logsProvisioner.LogsEnabled(app)
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, errors.New(doc)
		}
	}
	args.AppName = app.Name
	return logService.List(ctx, args)
}

type Filter struct {
	Name        string
	NameMatches string
	Platform    string
	TeamOwner   string
	UserOwner   string
	Pool        string
	Pools       []string
	Statuses    []string
	Locked      bool
	Tags        []string
	Extra       map[string][]string
}

func (f *Filter) IsEmpty() bool {
	if f == nil {
		return true
	}
	return reflect.DeepEqual(f, &Filter{})
}

func (f *Filter) ExtraIn(name string, value string) {
	if f.Extra == nil {
		f.Extra = make(map[string][]string)
	}
	f.Extra[name] = append(f.Extra[name], value)
}

func (f *Filter) Query() bson.M {
	if f == nil {
		return bson.M{}
	}
	query := bson.M{}
	if f.Extra != nil {
		var orBlock []bson.M
		for field, values := range f.Extra {
			orBlock = append(orBlock, bson.M{
				field: bson.M{"$in": values},
			})
		}
		query["$or"] = orBlock
	}
	if f.NameMatches != "" {
		query["name"] = bson.M{"$regex": f.NameMatches}
	}
	if f.Name != "" {
		query["name"] = f.Name
	}
	if f.TeamOwner != "" {
		query["teamowner"] = f.TeamOwner
	}
	if f.Platform != "" {
		parts := strings.SplitN(f.Platform, ":", 2)
		query["framework"] = parts[0]
		if len(parts) == 2 {
			v := parts[1]
			if v == "latest" {
				query["$and"] = []bson.M{
					{"$or": []bson.M{
						{"platformversion": bson.M{"$in": []string{"latest", ""}}},
						{"platformversion": bson.M{"$exists": false}},
					}},
				}
			} else {
				query["platformversion"] = v
			}
		}
	}
	if f.UserOwner != "" {
		query["owner"] = f.UserOwner
	}
	if f.Pool != "" {
		query["pool"] = f.Pool
	}
	if f.Locked {
		query["lock.locked"] = true
	}
	if len(f.Pools) > 0 {
		query["pool"] = bson.M{"$in": f.Pools}
	}
	tags := processTags(f.Tags)
	if len(tags) > 0 {
		query["tags"] = bson.M{"$all": tags}
	}
	return query
}

type AppUnitsResponse struct {
	Units []provision.Unit
	Err   error
}

func Units(ctx context.Context, apps []App) (map[string]AppUnitsResponse, error) {
	poolProvMap := map[string]provision.Provisioner{}
	provMap := map[provision.Provisioner][]provision.App{}
	for i, a := range apps {
		prov, ok := poolProvMap[a.Pool]
		if !ok {
			var err error
			prov, err = a.getProvisioner()
			if err != nil {
				return nil, err
			}
			poolProvMap[a.Pool] = prov
		}
		provMap[prov] = append(provMap[prov], &apps[i])
	}
	type parallelRsp struct {
		provApps []provision.App
		units    []provision.Unit
		err      error
	}
	rspCh := make(chan parallelRsp, len(provMap))
	wg := sync.WaitGroup{}
	for prov, provApps := range provMap {
		wg.Add(1)
		prov := prov
		provApps := provApps
		go func() {
			defer wg.Done()
			units, err := prov.Units(ctx, provApps...)
			rspCh <- parallelRsp{
				units:    units,
				err:      err,
				provApps: provApps,
			}
		}()
	}
	wg.Wait()
	close(rspCh)
	appUnits := map[string]AppUnitsResponse{}
	for pRsp := range rspCh {
		if pRsp.err != nil {
			for _, a := range pRsp.provApps {
				rsp := appUnits[a.GetName()]
				rsp.Err = errors.Wrap(pRsp.err, "unable to list app units")
				appUnits[a.GetName()] = rsp
			}
			continue
		}
		for _, u := range pRsp.units {
			rsp := appUnits[u.AppName]
			rsp.Units = append(rsp.Units, u)
			appUnits[u.AppName] = rsp
		}
	}
	return appUnits, nil
}

// List returns the list of apps filtered through the filter parameter.
func List(ctx context.Context, filter *Filter) ([]App, error) {
	apps := []App{}
	query := filter.Query()
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Apps().Find(query).All(&apps)
	conn.Close()
	if err != nil {
		return nil, err
	}
	for i := range apps {
		apps[i].ctx = ctx
	}
	if filter != nil && len(filter.Statuses) > 0 {
		appsProvisionerMap := make(map[string][]provision.App)
		var prov provision.Provisioner
		for i := range apps {
			a := &apps[i]
			prov, err = a.getProvisioner()
			if err != nil {
				return nil, err
			}
			appsProvisionerMap[prov.GetName()] = append(appsProvisionerMap[prov.GetName()], a)
		}
		var provisionApps []provision.App
		for provName, apps := range appsProvisionerMap {
			prov, err = provision.Get(provName)
			if err != nil {
				return nil, err
			}
			if filterProv, ok := prov.(provision.AppFilterProvisioner); ok {
				apps, err = filterProv.FilterAppsByUnitStatus(ctx, apps, filter.Statuses)
				if err != nil {
					return nil, err
				}
			}
			provisionApps = append(provisionApps, apps...)
		}
		for i := range provisionApps {
			apps[i] = *(provisionApps[i].(*App))
		}
		apps = apps[:len(provisionApps)]
	}
	err = loadCachedAddrsInApps(ctx, apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func appRouterAddrKey(appName, routerName string) string {
	return strings.Join([]string{"app-router-addr", appName, routerName}, "\x00")
}

func loadCachedAddrsInApps(ctx context.Context, apps []App) error {
	keys := make([]string, 0, len(apps))
	for i := range apps {
		a := &apps[i]
		a.Routers = a.GetRouters()
		for j := range a.Routers {
			keys = append(keys, appRouterAddrKey(a.Name, a.Routers[j].Name))
		}
	}
	entries, err := servicemanager.AppCache.List(ctx, keys...)
	if err != nil {
		return err
	}
	entryMap := make(map[string]cache.CacheEntry, len(entries))
	for _, e := range entries {
		entryMap[e.Key] = e
	}
	for i := range apps {
		a := &apps[i]
		hasEmpty := false
		for j := range apps[i].Routers {
			entry := entryMap[appRouterAddrKey(a.Name, a.Routers[j].Name)]
			a.Routers[j].Address = entry.Value
			if entry.Value == "" {
				hasEmpty = true
			}
		}
		if hasEmpty {
			GetAppRouterUpdater().update(a)
		}
	}
	return nil
}

func (app *App) hasMultipleVersions(ctx context.Context) (bool, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return false, err
	}
	versionProv, isVersionProv := prov.(provision.VersionsProvisioner)
	if !isVersionProv {
		return false, nil
	}
	versions, err := versionProv.DeployedVersions(ctx, app)
	if err != nil {
		return false, err
	}
	return len(versions) > 1, nil
}

// Swap calls the Router.Swap and updates the app.CName in the database.
func Swap(ctx context.Context, app1, app2 *App, cnameOnly bool) error {
	app1Multiple, err := app1.hasMultipleVersions(ctx)
	if err != nil {
		return err
	}
	app2Multiple, err := app2.hasMultipleVersions(ctx)
	if err != nil {
		return err
	}
	if app1Multiple || app2Multiple {
		return ErrSwapMultipleVersions
	}
	a1Routers := app1.GetRouters()
	a2Routers := app2.GetRouters()
	if len(a1Routers) != 1 || len(a2Routers) != 1 {
		return ErrSwapMultipleRouters
	}

	if a1Routers[0].Name != a2Routers[0].Name {
		return ErrSwapDifferentRouters
	}

	r, err := router.Get(ctx, a1Routers[0].Name)
	if err != nil {
		return err
	}

	if cnameOnly && len(app1.CName) == 0 && len(app2.CName) == 0 {
		return ErrSwapNoCNames
	}

	_, isRouterV2 := r.(router.RouterV2)
	if !cnameOnly && isRouterV2 {
		return ErrSwapDeprecated
	}

	// router v2 swap with rebuild with PreserveOldCNames
	if !isRouterV2 {
		err = r.Swap(ctx, app1, app2, cnameOnly)
		if err != nil {
			return err
		}
	}

	return action.NewPipeline(
		&swapCNamesInDatabaseAction,
		&swapReEnsureBackendsAction,
	).Execute(ctx, app1, app2)
}

// Start starts the app calling the provisioner.Start method and
// changing the units state to StatusStarted.
func (app *App) Start(ctx context.Context, w io.Writer, process, versionStr string) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("\n ---> Starting the process %q", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Starting the app %q", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(versionStr)
	if err != nil {
		return err
	}
	err = prov.Start(ctx, app, process, version, w)
	if err != nil {
		log.Errorf("[start] error on start the app %s - %s", app.Name, err)
		return newErrorWithLog(err, app, "start")
	}
	rebuild.RoutesRebuildOrEnqueueWithProgress(app.Name, w)
	return err
}

func (app *App) SetUpdatePlatform(check bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"updateplatform": check}},
	)
}

func (app *App) GetUpdatePlatform() bool {
	return app.UpdatePlatform
}

func (app *App) RegisterUnit(ctx context.Context, unitId string, customData map[string]interface{}) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	err = prov.RegisterUnit(ctx, app, unitId, customData)
	if err != nil {
		return err
	}
	units, err := prov.Units(ctx, app)
	if err != nil {
		return err
	}
	for i := range units {
		if units[i].GetID() == unitId {
			err = app.BindUnit(&units[i])
			break
		}
	}
	return err
}

func (app *App) AddRouter(appRouter appTypes.AppRouter) error {
	for _, r := range app.GetRouters() {
		if appRouter.Name == r.Name {
			return ErrRouterAlreadyLinked
		}
	}
	r, err := router.Get(app.ctx, appRouter.Name)
	if err != nil {
		return err
	}
	if _, ok := r.(router.RouterV2); ok {
		err = router.Store(app.GetName(), app.GetName(), r.GetType())
		if err != nil {
			return err
		}

		// skip rebuild routes task if app has no units available
		if app.available() {
			_, err = rebuild.RebuildRoutesInRouter(app.ctx, appRouter, rebuild.RebuildRoutesOpts{
				App:  app,
				Wait: true,
			})
		}
	} else if optsRouter, ok := r.(router.OptsRouter); ok {
		defer rebuild.RoutesRebuildOrEnqueue(app.Name)
		err = optsRouter.AddBackendOpts(app.ctx, app, appRouter.Opts)
	} else {
		defer rebuild.RoutesRebuildOrEnqueue(app.Name)
		err = r.AddBackend(app.ctx, app)
	}
	if err != nil {
		return err
	}
	routers := append(app.GetRouters(), appRouter)
	err = app.updateRoutersDB(routers)
	if err != nil {
		rollbackErr := r.RemoveBackend(app.ctx, app)
		if rollbackErr != nil {
			log.Errorf("unable to remove router backend rolling back add router: %v", rollbackErr)
		}
		return err
	}
	return nil
}

func (app *App) UpdateRouter(appRouter appTypes.AppRouter) error {
	var existing *appTypes.AppRouter
	routers := app.GetRouters()
	for i, r := range routers {
		if r.Name == appRouter.Name {
			existing = &routers[i]
			break
		}
	}
	if existing == nil {
		return &router.ErrRouterNotFound{Name: appRouter.Name}
	}
	r, err := router.Get(app.ctx, appRouter.Name)
	if err != nil {
		return err
	}
	_, isRouterV2 := r.(router.RouterV2)
	optsRouter, ok := r.(router.OptsRouter)
	if !ok {
		return errors.Errorf("updating is not supported by router %q", appRouter.Name)
	}
	oldOpts := existing.Opts
	existing.Opts = appRouter.Opts
	err = app.updateRoutersDB(routers)
	if err != nil {
		return err
	}
	if isRouterV2 {
		_, err = rebuild.RebuildRoutesInRouter(app.ctx, appRouter, rebuild.RebuildRoutesOpts{
			App:  app,
			Wait: true,
		})
		if err != nil {
			return err
		}
	} else {
		err = optsRouter.UpdateBackendOpts(app.ctx, app, appRouter.Opts)
		if err != nil {
			existing.Opts = oldOpts
			rollbackErr := app.updateRoutersDB(routers)
			if rollbackErr != nil {
				log.Errorf("unable to update router opts in db rolling back update router: %v", rollbackErr)
			}
			return err
		}
	}
	return nil
}

func (app *App) RemoveRouter(name string) error {
	removed := false
	routers := app.GetRouters()
	for i, r := range routers {
		if r.Name == name {
			removed = true
			// Preserve order
			routers = append(routers[:i], routers[i+1:]...)
			break
		}
	}
	if !removed {
		return &router.ErrRouterNotFound{Name: name}
	}
	r, err := router.Get(app.ctx, name)
	if err != nil {
		return err
	}
	err = app.updateRoutersDB(routers)
	if err != nil {
		return err
	}
	err = r.RemoveBackend(app.ctx, app)
	if err != nil {
		log.Errorf("unable to remove router backend: %v", err)
	}
	return nil
}

func (app *App) updateRoutersDB(routers []appTypes.AppRouter) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app.Routers = routers
	app.Router = ""
	app.RouterOpts = nil
	return conn.Apps().Update(bson.M{"name": app.Name}, bson.M{
		"$set": bson.M{
			"routers":    app.Routers,
			"router":     app.Router,
			"routeropts": app.RouterOpts,
		},
	})
}

func (app *App) GetRouters() []appTypes.AppRouter {
	routers := append([]appTypes.AppRouter{}, app.Routers...)
	if app.Router != "" {
		for _, r := range routers {
			if r.Name == app.Router {
				return routers
			}
		}
		routers = append([]appTypes.AppRouter{{
			Name: app.Router,
			Opts: app.RouterOpts,
		}}, routers...)
	}
	return routers
}

func (app *App) GetRoutersWithAddr() ([]appTypes.AppRouter, error) {
	routers := app.GetRouters()
	multi := tsuruErrors.NewMultiError()
	for i := range routers {
		routerName := routers[i].Name
		r, planRouter, err := router.GetWithPlanRouter(app.ctx, routerName)
		if err != nil {
			multi.Add(err)
			continue
		}
		addr, err := r.Addr(app.ctx, app)
		if err != nil {
			if errors.Cause(err) == router.ErrBackendNotFound {
				routers[i].Status = "not ready"
				continue
			}
			multi.Add(err)
			continue
		}
		if statusRouter, ok := r.(router.StatusRouter); ok {
			status, stErr := statusRouter.GetBackendStatus(app.ctx, app, "")
			if stErr != nil {
				multi.Add(stErr)
				continue
			}
			routers[i].Status = string(status.Status)
			routers[i].StatusDetail = status.Detail
		}
		if prefixRouter, ok := r.(router.PrefixRouter); ok {
			addrs, aErr := prefixRouter.Addresses(app.ctx, app)
			if aErr != nil {
				multi.Add(aErr)
				continue
			}
			routers[i].Addresses = addrs
		}
		servicemanager.AppCache.Create(app.ctx, cache.CacheEntry{
			Key:   appRouterAddrKey(app.Name, routerName),
			Value: addr,
		})
		routers[i].Address = addr
		routers[i].Type = planRouter.Type
	}
	return routers, multi.ToError()
}

func (app *App) MetricEnvs() (map[string]string, error) {
	bsContainer, err := nodecontainer.LoadNodeContainer(app.GetPool(), nodecontainer.BsDefaultName)
	if err != nil {
		return nil, err
	}
	envs := bsContainer.EnvMap()
	for envName := range envs {
		if !strings.HasPrefix(envName, "METRICS_") {
			delete(envs, envName)
		}
	}
	return envs, nil
}

func (app *App) Shell(opts provision.ExecOptions) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	execProv, ok := prov.(provision.ExecutableProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "running shell"}
	}
	opts.App = app
	opts.Cmds = cmdsForExec("[ $(command -v bash) ] && exec bash -l || exec sh -l")
	return execProv.ExecuteCommand(app.ctx, opts)
}

func (app *App) SetCertificate(name, certificate, key string) error {
	err := app.validateNameForCert(name)
	if err != nil {
		return err
	}
	cert, err := tls.X509KeyPair([]byte(certificate), []byte(key))
	if err != nil {
		return err
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}
	err = x509Cert.VerifyHostname(name)
	if err != nil {
		return err
	}
	addedAny := false
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(app.ctx, appRouter.Name)
		if err != nil {
			return err
		}
		tlsRouter, ok := r.(router.TLSRouter)
		if !ok {
			continue
		}
		addedAny = true
		err = tlsRouter.AddCertificate(app.ctx, app, name, certificate, key)
		if err != nil {
			return err
		}
	}
	if !addedAny {
		return errors.New("no router with tls support")
	}
	return nil
}

func (app *App) RemoveCertificate(name string) error {
	err := app.validateNameForCert(name)
	if err != nil {
		return err
	}
	removedAny := false
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(app.ctx, appRouter.Name)
		if err != nil {
			return err
		}
		tlsRouter, ok := r.(router.TLSRouter)
		if !ok {
			continue
		}
		removedAny = true
		err = tlsRouter.RemoveCertificate(app.ctx, app, name)
		if err != nil {
			return err
		}
	}
	if !removedAny {
		return errors.New("no router with tls support")
	}
	return nil
}

func (app *App) validateNameForCert(name string) error {
	addrs, err := app.GetAddresses()
	if err != nil {
		return err
	}
	hasName := false
	for _, n := range append(addrs, app.CName...) {
		if n == name {
			hasName = true
			break
		}
	}
	if !hasName {
		return errors.New("invalid name")
	}
	return nil
}

func (app *App) GetCertificates() (map[string]map[string]string, error) {
	addrs, err := app.GetAddresses()
	if err != nil {
		return nil, err
	}
	names := append(addrs, app.CName...)
	allCertificates := make(map[string]map[string]string)
	for _, appRouter := range app.GetRouters() {
		certificates := make(map[string]string)
		r, err := router.Get(app.ctx, appRouter.Name)
		if err != nil {
			return nil, err
		}
		tlsRouter, ok := r.(router.TLSRouter)
		if !ok {
			continue
		}
		for _, n := range names {
			cert, err := tlsRouter.GetCertificate(app.ctx, app, n)
			if err != nil && err != router.ErrCertificateNotFound {
				return nil, errors.Wrapf(err, "error in router %q", appRouter.Name)
			}
			certificates[n] = cert
		}
		allCertificates[appRouter.Name] = certificates
	}
	if len(allCertificates) == 0 {
		return nil, errors.New("no router with tls support")
	}
	return allCertificates, nil
}

func (app *App) RoutableAddresses(ctx context.Context) ([]appTypes.RoutableAddresses, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	return prov.RoutableAddresses(ctx, app)
}

func (app *App) withLogWriter(w io.Writer) io.Writer {
	logWriter := &LogWriter{AppName: app.Name}
	if w != nil {
		w = io.MultiWriter(w, logWriter)
	} else {
		w = logWriter
	}
	return w
}

func RenameTeam(ctx context.Context, oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	filter := &Filter{}
	filter.ExtraIn("teams", oldName)
	filter.ExtraIn("teamowner", oldName)
	apps, err := List(ctx, filter)
	if err != nil {
		return err
	}
	for _, a := range apps {
		var evt *event.Event
		evt, err = event.NewInternal(&event.Opts{
			Target:       event.Target{Type: event.TargetTypeApp, Value: a.Name},
			InternalKind: "team rename",
			Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, a.Name)),
		})
		if err != nil {
			return errors.Wrap(err, "unable to create event")
		}
		defer evt.Abort()
	}
	bulk := conn.Apps().Bulk()
	for _, a := range apps {
		if a.TeamOwner == oldName {
			a.TeamOwner = newName
		}
		for i, team := range a.Teams {
			if team == oldName {
				a.Teams[i] = newName
			}
		}
		bulk.Update(bson.M{"name": a.Name}, a)
	}
	_, err = bulk.Run()
	return err
}

func (app *App) GetHealthcheckData() (routerTypes.HealthcheckData, error) {
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(app.ctx, app)
	if err != nil {
		if err == appTypes.ErrNoVersionsAvailable {
			err = nil
		}
		return routerTypes.HealthcheckData{}, err
	}
	yamlData, err := version.TsuruYamlData()
	if err != nil {
		return routerTypes.HealthcheckData{}, err
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return routerTypes.HealthcheckData{}, err
	}
	if hcProv, ok := prov.(provision.HCProvisioner); ok {
		if hcProv.HandlesHC() {
			return routerTypes.HealthcheckData{
				TCPOnly: true,
			}, nil
		}
	}
	return yamlData.ToRouterHC(), nil
}

func validateEnv(envName string) error {
	if !envVarNameRegexp.MatchString(envName) {
		return &tsuruErrors.ValidationError{Message: fmt.Sprintf("Invalid environment variable name: '%s'", envName)}
	}
	return nil
}

func (app *App) SetRoutable(ctx context.Context, version appTypes.AppVersion, isRoutable bool) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	rprov, ok := prov.(provision.VersionsProvisioner)
	if !ok {
		return errors.Errorf("provisioner %v does not support setting versions routable", prov.GetName())
	}
	return rprov.ToggleRoutable(ctx, app, version, isRoutable)
}

func (app *App) DeployedVersions() ([]int, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	if rprov, ok := prov.(provision.VersionsProvisioner); ok {
		return rprov.DeployedVersions(app.ctx, app)
	}
	return nil, ErrNoVersionProvisioner
}

func (app *App) getVersion(ctx context.Context, version string) (appTypes.AppVersion, error) {
	versionProv, v, err := app.explicitVersion(version)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}

	versions, err := versionProv.DeployedVersions(ctx, app)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return servicemanager.AppVersion.LatestSuccessfulVersion(app.ctx, app)
	}
	if len(versions) > 1 {
		return nil, errors.Errorf("more than one version deployed, you must select one")
	}

	return servicemanager.AppVersion.VersionByImageOrVersion(app.ctx, app, strconv.Itoa(versions[0]))
}

func (app *App) getVersionAllowNil(version string) (appTypes.AppVersion, error) {
	_, v, err := app.explicitVersion(version)
	return v, err
}

func (app *App) explicitVersion(version string) (provision.VersionsProvisioner, appTypes.AppVersion, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, nil, err
	}
	versionProv, isVersionProv := prov.(provision.VersionsProvisioner)

	if !isVersionProv {
		latest, err := servicemanager.AppVersion.LatestSuccessfulVersion(app.ctx, app)
		if err != nil {
			return nil, nil, err
		}
		if version != "" && version != "0" {
			v, err := servicemanager.AppVersion.VersionByImageOrVersion(app.ctx, app, version)
			if err != nil {
				return nil, nil, err
			}
			if latest.Version() != v.Version() {
				return nil, nil, errors.Errorf("explicit version not supported for provisioner %v", prov.GetName())
			}
		}
		return nil, latest, nil
	}

	if version != "" && version != "0" {
		v, err := servicemanager.AppVersion.VersionByImageOrVersion(app.ctx, app, version)
		return versionProv, v, err
	}

	return versionProv, nil, nil
}

func (app *App) ListTags() []string {
	return app.Tags
}

func (app *App) GetMetadata() appTypes.Metadata {
	return app.Metadata
}

func (app *App) AutoScaleInfo() ([]provision.AutoScaleSpec, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetAutoScale(app.ctx, app)
}

func (app *App) VerticalAutoScaleRecommendations() ([]provision.RecommendedResources, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetVerticalAutoScaleRecommendations(app.ctx, app)
}

func (app *App) UnitsMetrics() ([]provision.UnitMetric, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	metricsProv, ok := prov.(provision.MetricsProvisioner)
	if !ok {
		return nil, nil
	}
	return metricsProv.UnitsMetrics(app.ctx, app)
}

func (app *App) AutoScale(spec provision.AutoScaleSpec) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.SetAutoScale(app.ctx, app, spec)
}

func (app *App) RemoveAutoScale(process string) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.RemoveAutoScale(app.ctx, app, process)
}

func envInSet(envName string, envs []bind.EnvVar) bool {
	for _, e := range envs {
		if e.Name == envName {
			return true
		}
	}
	return false
}

func (app *App) GetRegistry() (imgTypes.ImageRegistry, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return "", err
	}
	registryProv, ok := prov.(provision.MultiRegistryProvisioner)
	if !ok {
		return "", nil
	}
	return registryProv.RegistryForApp(app.ctx, app)
}
