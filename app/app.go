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
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
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
	bindTypes "github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/cache"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	routerTypes "github.com/tsuru/tsuru/types/router"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	"github.com/tsuru/tsuru/validation"
)

var (
	AuthScheme auth.Scheme
)

var (
	ErrAlreadyHaveAccess = errors.New("team already have access to this app")
	ErrNoAccess          = errors.New("team does not have access to this app")
	ErrCannotOrphanApp   = errors.New("cannot revoke access from this team, as it's the unique team with access to the app")
	ErrDisabledPlatform  = errors.New("Disabled Platform, only admin users can create applications with the platform")

	ErrRouterAlreadyLinked = errors.New("router already linked to this app")

	ErrNoVersionProvisioner = errors.New("The current app provisioner does not support multiple versions handling")
	ErrKillUnitProvisioner  = errors.New("The current app provisioner does not support killing a unit")
	ErrSwapMultipleVersions = errors.New("swapping apps with multiple versions is not allowed")
	ErrSwapMultipleRouters  = errors.New("swapping apps with multiple routers is not supported")
	ErrSwapDifferentRouters = errors.New("swapping apps with different routers is not supported")
	ErrSwapNoCNames         = errors.New("no cnames to swap")
	ErrSwapDeprecated       = errors.New("swapping without cnameOnly is deprecated")
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
	defaultAppDir = "/home/application/current"

	routerNone = "none"
)

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App appTypes.App

var (
	_ provision.App        = &App{}
	_ rebuild.RebuildApp   = &App{}
	_ quota.QuotaItemInUse = &App{}
)

func (app *App) getBuilder(ctx context.Context) (builder.Builder, error) {
	p, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	return builder.GetForProvisioner(p)
}

func internalAddresses(ctx context.Context, app *App) ([]appTypes.AppInternalAddress, error) {
	provisioner, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}

	if interAppProvisioner, ok := provisioner.(provision.InterAppProvisioner); ok {
		return interAppProvisioner.InternalAddresses(ctx, app)
	}

	return nil, nil
}

func (app *App) getProvisioner(ctx context.Context) (provision.Provisioner, error) {
	return pool.GetProvisionerForPool(ctx, app.Pool)
}

// Units returns the list of units.
func (app *App) Units(ctx context.Context) ([]provTypes.Unit, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return []provTypes.Unit{}, err
	}
	units, err := prov.Units(context.TODO(), app)
	if units == nil {
		// This is unusual but was done because previously this method didn't
		// return an error. This ensures we always return an empty list instead
		// of nil to preserve compatibility with old clients.
		units = []provTypes.Unit{}
	}
	return units, err
}

// AppInfo returns a agregated format of app
func AppInfo(ctx context.Context, app *App) (*appTypes.AppInfo, error) {
	var errMsgs []string
	result := &appTypes.AppInfo{
		Name:        app.Name,
		Description: app.Description,
		Platform:    app.Platform,
		Teams:       app.Teams,
		Plan:        &app.Plan,
		CName:       app.CName,
		Owner:       app.Owner,
		Pool:        app.Pool,
		Deploys:     app.Deploys,
		TeamOwner:   app.TeamOwner,
		Lock:        app.Lock,
		Tags:        app.Tags,
		Metadata:    app.Metadata,
	}

	if version := app.GetPlatformVersion(); version != "latest" {
		result.Platform = fmt.Sprintf("%s:%s", app.Platform, version)
	}
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get provisioner name: %+v", err))
	}
	if prov != nil {
		provisionerName := prov.GetName()
		result.Provisioner = provisionerName
		cluster, clusterErr := servicemanager.Cluster.FindByPool(ctx, provisionerName, app.Pool)
		if clusterErr != nil && clusterErr != provTypes.ErrNoCluster {
			errMsgs = append(errMsgs, fmt.Sprintf("unable to get cluster name: %+v", clusterErr))
		}
		if cluster != nil {
			result.Cluster = cluster.Name
		}
	}
	units, err := app.Units(ctx)
	result.Units = units
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to list app units: %+v", err))
	}

	routers, err := app.GetRoutersWithAddr(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get app addresses: %+v", err))
	}
	if len(routers) > 0 {
		result.IP = routers[0].Address
		result.Router = routers[0].Name
		result.RouterOpts = routers[0].Opts
	}
	result.Routers = routers

	if len(app.Processes) > 0 {
		result.Processes = app.Processes
	}

	q, err := app.GetQuota(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get app quota: %+v", err))
	}
	if q != nil {
		result.Quota = q
	}
	internalAddresses, err := internalAddresses(ctx, app)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get app cluster internal addresses: %+v", err))
	}

	if len(internalAddresses) > 0 {
		result.InternalAddresses = internalAddresses
	}
	autoscale, err := app.AutoScaleInfo(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale info: %+v", err))
	}
	if autoscale != nil {
		result.Autoscale = autoscale
	}
	autoscaleRec, err := app.VerticalAutoScaleRecommendations(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale recommendation info: %+v", err))
	}
	if autoscaleRec != nil {
		result.AutoscaleRecommendation = autoscaleRec
	}
	unitMetrics, err := app.UnitsMetrics(ctx)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get units metrics: %+v", err))
	}
	if unitMetrics != nil {
		result.UnitsMetrics = unitMetrics
	}
	volumeBinds, err := servicemanager.Volume.BindsForApp(ctx, nil, app.Name)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get volume binds: %+v", err))
	}
	if volumeBinds != nil {
		result.VolumeBinds = volumeBinds
	}
	sis, err := service.GetServiceInstancesBoundToApp(ctx, app.Name)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get service instance bound to app: %+v", err))
	}
	binds := make([]bindTypes.ServiceInstanceBind, 0)
	for _, si := range sis {
		binds = append(binds, bindTypes.ServiceInstanceBind{
			Service:  si.ServiceName,
			Instance: si.Name,
			Plan:     si.PlanName,
		})
	}
	result.ServiceInstanceBinds = binds
	if len(errMsgs) > 0 {
		result.Error = strings.Join(errMsgs, "\n")
	}
	return result, nil
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
	return &app, err
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of the following steps:
//
//  1. Save the app in the database
//  2. Provision the app using the provisioner
func CreateApp(ctx context.Context, app *App, user *auth.User) error {
	if _, err := GetByName(ctx, app.GetName()); err != appTypes.ErrAppNotFound {
		if err != nil {
			return errors.WithMessage(err, "unable to check if app already exists")
		}
		return &appTypes.AppCreationError{Err: ErrAppAlreadyExists, App: app.GetName()}
	}
	var err error
	err = app.SetPool(ctx)
	if err != nil {
		return err
	}
	appPool, err := pool.GetPoolByName(ctx, app.GetPool())
	if err != nil {
		return err
	}

	var plan *appTypes.Plan
	if app.Plan.Name == "" {
		plan, err = appPool.GetDefaultPlan(ctx)
	} else {
		plan, err = servicemanager.Plan.FindByName(ctx, app.Plan.Name)
	}
	if err != nil {
		return err
	}
	app.Plan = *plan
	err = app.configureCreateRouters(ctx)
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
	app.pruneProcesses()
	err = app.validateNew(ctx)
	if err != nil {
		return err
	}
	actions := []*action.Action{
		&reserveTeamApp,
		&reserveUserApp,
		&insertApp,
		&exportEnvironmentsAction,
		&provisionApp,
	}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(ctx, app, user)
	if err != nil {
		return &appTypes.AppCreationError{App: app.Name, Err: err}
	}
	return nil
}

func (app *App) configureCreateRouters(ctx context.Context) error {
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
		appPool, err = pool.GetPoolByName(ctx, app.GetPool())
		if err != nil {
			return err
		}
		app.Router, err = appPool.GetDefaultRouter(ctx)
	} else {
		_, err = router.Get(ctx, app.Router)
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
func (app *App) Update(ctx context.Context, args UpdateAppArgs) (err error) {
	description := args.UpdateData.Description
	poolName := args.UpdateData.Pool
	teamOwner := args.UpdateData.TeamOwner
	platform := args.UpdateData.Platform
	tags := processTags(args.UpdateData.Tags)
	oldApp := *app

	oldPlan, err := json.Marshal(oldApp.Plan)
	if err != nil {
		return err
	}

	if description != "" {
		app.Description = description
	}
	if poolName != "" {
		app.Pool = poolName
		_, err = app.getPoolForApp(ctx, app.Pool)
		if err != nil {
			return err
		}
	}
	newProv, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	oldProv, err := oldApp.getProvisioner(ctx)
	if err != nil {
		return err
	}
	if args.UpdateData.Plan.Name != "" {
		plan, errFind := servicemanager.Plan.FindByName(ctx, args.UpdateData.Plan.Name)
		if errFind != nil {
			return errFind
		}
		app.Plan = *plan
	}
	override := args.UpdateData.Plan.Override
	if override == nil {
		override = &appTypes.PlanOverride{}
	}
	app.Plan.MergeOverride(*override)

	newPlan, err := json.Marshal(app.Plan)
	if err != nil {
		return err
	}

	if teamOwner != "" {
		team, errTeam := servicemanager.Team.FindByName(ctx, teamOwner)
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

	processesHasChanged, err := app.updateProcesses(ctx, args.UpdateData.Processes)
	if err != nil {
		return err
	}

	app.Metadata.Update(args.UpdateData.Metadata)
	if platform != "" {
		var p, v string
		p, v, err = app.getPlatformNameAndVersion(ctx, platform)
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
	err = app.validate(ctx)
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
			rebuildErr := rebuild.RebuildRoutesWithAppName(app.Name, args.Writer)
			if rebuildErr != nil {
				log.Errorf("Could not rebuild route: %s", rebuildErr.Error())
			}

		}()
		err = validateVolumes(ctx, app)
		if err != nil {
			return err
		}
		actions = append(actions,
			&provisionAppNewProvisioner,
			&provisionAppAddUnits,
			&destroyAppOldProvisioner)
	} else if string(newPlan) != string(oldPlan) && args.ShouldRestart {
		actions = append(actions, &restartApp)
	} else if app.Pool != oldApp.Pool && !updatePipelineAdded {
		actions = append(actions, &restartApp)
	} else if processesHasChanged && args.ShouldRestart {
		actions = append(actions, &restartApp)
	} else if !reflect.DeepEqual(app.Metadata, oldApp.Metadata) && args.ShouldRestart {
		actions = append(actions, &restartApp)
	}
	return action.NewPipeline(actions...).Execute(ctx, app, &oldApp, args.Writer)
}

func (app *App) updateProcesses(ctx context.Context, new []appTypes.Process) (changed bool, err error) {
	if len(app.Processes) == 0 && len(new) == 0 {
		return false, nil
	}

	oldProcesses, err := json.Marshal(app.Processes)
	if err != nil {
		return false, errors.WithMessage(err, "could not serialize app process")
	}

	positionByName := map[string]*int{}
	for i, p := range app.Processes {
		positionByName[p.Name] = func(n int) *int { return &n }(i)
	}

	for _, p := range new {
		if p.Plan != "" && p.Plan != "$default" {
			_, err = servicemanager.Plan.FindByName(ctx, p.Plan)
			if err != nil {
				return false, errors.WithMessagef(err, "could not find plan %q", p.Plan)
			}
		}

		pos := positionByName[p.Name]
		if pos == nil {
			app.Processes = append(app.Processes, p)
			continue
		}

		if p.Plan != "" {
			app.Processes[*pos].Plan = p.Plan
		}
		app.Processes[*pos].Metadata.Update(p.Metadata)

	}

	app.pruneProcesses()

	newProcesses, err := json.Marshal(app.Processes)
	if err != nil {
		return false, errors.WithMessage(err, "could not serialize app process")
	}

	return string(oldProcesses) != string(newProcesses), nil
}

func (app *App) pruneProcesses() {
	updated := []appTypes.Process{}
	for _, process := range app.Processes {
		if process.Plan == "$default" {
			process.Plan = ""
		}

		if !process.Empty() {
			updated = append(updated, process)
		}
	}

	sort.Slice(updated, func(i, j int) bool {
		return updated[i].Name < updated[j].Name
	})

	app.Processes = updated

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
	reg, err := app.GetRegistry(ctx)
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
func (app *App) unbind(ctx context.Context, evt *event.Event, requestID string) error {
	instances, err := service.GetServiceInstancesBoundToApp(ctx, app.Name)
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
		err = instance.UnbindApp(ctx, service.UnbindAppArgs{
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

func (app *App) unbindVolumes(ctx context.Context) error {
	volumes, err := servicemanager.Volume.ListByApp(ctx, app.Name)
	if err != nil {
		return errors.Wrap(err, "Unable to list volumes for unbind")
	}
	for _, v := range volumes {
		var binds []volumeTypes.VolumeBind
		binds, err = servicemanager.Volume.Binds(ctx, &v)
		if err != nil {
			return errors.Wrap(err, "Unable to list volume binds for unbind")
		}
		for _, b := range binds {
			err = servicemanager.Volume.UnbindApp(ctx, &volumeTypes.BindOpts{
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
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	err = registry.RemoveAppImages(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove images from registry for app %s: %s", appName, err)
	}

	err = servicemanager.AppVersion.DeleteVersions(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove image names from storage for app %s: %s", appName, err)
	}
	err = app.unbind(ctx, evt, requestID)
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
		if err != nil && err != router.ErrBackendNotFound {
			logErr("Failed to remove router backend", err)
		}
	}
	err = app.unbindVolumes(ctx)
	if err != nil {
		logErr("Unable to unbind volumes", err)
	}
	owner, err := auth.GetUserByEmail(app.Owner)
	if err == nil {
		err = servicemanager.UserQuota.Inc(ctx, owner, -1)
	}
	if err != nil {
		logErr("Unable to release app quota", err)
	}

	err = servicemanager.TeamQuota.Inc(ctx, &authTypes.Team{Name: app.TeamOwner}, -1)
	if err != nil {
		logErr("Unable to release team quota", err)
	}
	if plog, ok := servicemanager.LogService.(appTypes.AppLogServiceProvision); ok {
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

	_, version, err := app.explicitVersion(ctx, versionStr)
	if err != nil {
		return err
	}
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}

	err = prov.DestroyVersion(ctx, app, version)
	if err != nil {
		logErr("Unable to destroy app in provisioner", err)
	}

	return nil
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (app *App) AddUnits(ctx context.Context, n uint, process, versionStr string, w io.Writer) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
	}

	err := app.ensureNoAutoscaler(ctx, process)
	if err != nil {
		return err
	}

	units, err := app.Units(ctx)
	if err != nil {
		return err
	}
	for _, u := range units {
		if u.Status == provTypes.UnitStatusStopped {
			return errors.New("Cannot add units to an app that has stopped units")
		}
	}
	version, err := app.getVersion(ctx, versionStr)
	if err != nil {
		return err
	}
	w = app.withLogWriter(w)
	err = action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
	).Execute(ctx, app, n, w, process, version)
	if err != nil {
		return newErrorWithLog(ctx, err, app, "add units")
	}
	err = rebuild.RebuildRoutesWithAppName(app.Name, w)
	if err != nil {
		return err
	}
	return nil
}

func (app *App) ensureNoAutoscaler(ctx context.Context, process string) error {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if ok {
		autoscales, err := autoscaleProv.GetAutoScale(ctx, app)
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
//  1. Remove units from the provisioner
//  2. Update quota
func (app *App) RemoveUnits(ctx context.Context, n uint, process, versionStr string, w io.Writer) error {
	err := app.ensureNoAutoscaler(ctx, process)
	if err != nil {
		return err
	}
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	w = app.withLogWriter(w)
	version, err := app.getVersion(ctx, versionStr)
	if err != nil {
		return err
	}
	err = prov.RemoveUnits(ctx, app, n, process, version, w)
	if err != nil {
		return newErrorWithLog(ctx, err, app, "remove units")
	}

	err = rebuild.RebuildRoutesWithAppName(app.Name, w)
	return err
}

func (app *App) KillUnit(ctx context.Context, unitName string, force bool) error {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	unitProv, ok := prov.(provision.KillUnitProvisioner)
	if !ok {
		return ErrKillUnitProvisioner
	}
	return unitProv.KillUnit(ctx, app, unitName, force)
}

type UpdateUnitsResult struct {
	ID    string
	Found bool
}

// available returns true if at least one of N units is started or unreachable.
func (app *App) available(ctx context.Context) bool {
	units, err := app.Units(ctx)
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
func (app *App) GetTeams(ctx context.Context) []authTypes.Team {
	t, _ := servicemanager.Team.FindByNames(ctx, app.Teams)
	return t
}

func (app *App) SetPool(ctx context.Context) error {
	poolName, err := app.getPoolForApp(ctx, app.Pool)
	if err != nil {
		return err
	}
	if poolName == "" {
		var p *pool.Pool
		p, err = pool.GetDefaultPool(ctx)
		if err != nil {
			return err
		}
		poolName = p.Name
	}
	app.Pool = poolName
	p, err := pool.GetPoolByName(ctx, poolName)
	if err != nil {
		return err
	}
	return app.validateTeamOwner(ctx, p)
}

func (app *App) getPoolForApp(ctx context.Context, poolName string) (string, error) {
	if poolName == "" {
		pools, err := pool.ListPoolsForTeam(ctx, app.TeamOwner)
		if err != nil {
			return "", err
		}
		if len(pools) > 1 {
			publicPools, err := pool.ListPublicPools(ctx)
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
	pool, err := pool.GetPoolByName(ctx, poolName)
	if err != nil {
		return "", err
	}
	return pool.Name, nil
}

// setEnv sets the given environment variable in the app.
func (app *App) setEnv(env bindTypes.EnvVar) {
	if app.Env == nil {
		app.Env = make(map[string]bindTypes.EnvVar)
	}
	app.Env[env.Name] = env
	if env.Public {
		servicemanager.LogService.Add(app.Name, fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru", "api")
	}
}

// validateNew checks app name format, pool and plan
func (app *App) validateNew(ctx context.Context) error {
	if !validation.ValidateName(app.Name) {
		msg := "Invalid app name, your app should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return app.validate(ctx)
}

// validate checks app pool and plan
func (app *App) validate(ctx context.Context) error {
	err := app.validatePool(ctx)
	if err != nil {
		return err
	}

	err = app.validateProcesses()
	if err != nil {
		return err
	}

	return app.validatePlan(ctx)
}

func (app *App) validatePlan(ctx context.Context) error {
	cpuBurst := app.Plan.CPUBurst
	planOverride := app.Plan.Override

	if cpuBurst == nil {
		cpuBurst = &appTypes.CPUBurst{}
	}

	if planOverride == nil {
		planOverride = &appTypes.PlanOverride{}
	}

	if (cpuBurst.MaxAllowed != 0) &&
		(planOverride.CPUBurst != nil) &&
		(*planOverride.CPUBurst > cpuBurst.MaxAllowed) {

		msg := fmt.Sprintf("CPU burst exceeds the maximum allowed by plan %q", app.Plan.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}

	pool, err := pool.GetPoolByName(ctx, app.Pool)
	if err != nil {
		return err
	}
	plans, err := pool.GetPlans(ctx)
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

func (app *App) validatePool(ctx context.Context) error {
	pool, err := pool.GetPoolByName(ctx, app.Pool)
	if err != nil {
		return err
	}
	err = app.validateTeamOwner(ctx, pool)
	if err != nil {
		return err
	}
	instances, err := service.GetServiceInstancesBoundToApp(ctx, app.Name)
	if err != nil {
		return err
	}
	if len(instances) > 0 {
		serviceNames := make([]string, len(instances))
		for i, instance := range instances {
			serviceNames[i] = instance.ServiceName
		}
		err = app.ValidateService(ctx, serviceNames...)
		if err != nil {
			return err
		}
	}

	return pool.ValidateRouters(ctx, app.GetRouters())
}

func (app *App) validateProcesses() error {
	namesUsed := map[string]bool{}

	for _, p := range app.Processes {
		if p.Name == "" {
			return &tsuruErrors.ValidationError{Message: "empty process name is not allowed"}
		}
		if namesUsed[p.Name] {
			msg := fmt.Sprintf("process %q is duplicated", p.Name)
			return &tsuruErrors.ValidationError{Message: msg}
		}

		namesUsed[p.Name] = true
	}

	return nil
}

func (app *App) validateTeamOwner(ctx context.Context, p *pool.Pool) error {
	_, err := servicemanager.Team.FindByName(ctx, app.TeamOwner)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	poolTeams, err := p.GetTeams(ctx)
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

func (app *App) ValidateService(ctx context.Context, services ...string) error {
	_, err := pool.GetPoolByName(ctx, app.Pool)
	if err != nil {
		return err
	}

	return pool.ValidatePoolService(ctx, app.Pool, services)
}

// InstanceEnvs returns a map of environment variables that belongs to the
// given service and service instance.
func (app *App) InstanceEnvs(serviceName, instanceName string) map[string]bindTypes.EnvVar {
	envs := make(map[string]bindTypes.EnvVar)
	for _, env := range app.ServiceEnvs {
		if env.ServiceName == serviceName && env.InstanceName == instanceName {
			envs[env.Name] = env.EnvVar
		}
	}
	return envs
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (app *App) Run(ctx context.Context, cmd string, w io.Writer, args provision.RunArgs) error {
	if !args.Isolated && !app.available(ctx) {
		return errors.New("App must be available to run non-isolated commands")
	}
	logWriter := LogWriter{AppName: app.Name, Source: "app-run"}
	logWriter.Async()
	defer logWriter.Close()
	logWriter.Write([]byte(fmt.Sprintf("running '%s'", cmd)))
	return app.run(ctx, cmd, io.MultiWriter(w, &logWriter), args)
}

func cmdsForExec(cmd string) []string {
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := fmt.Sprintf("[ -d %s ] && cd %s", defaultAppDir, defaultAppDir)
	return []string{"/bin/sh", "-c", fmt.Sprintf("%s; %s; %s", source, cd, cmd)}
}

func (app *App) run(ctx context.Context, cmd string, w io.Writer, args provision.RunArgs) error {
	prov, err := app.getProvisioner(ctx)
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
	units, err := app.Units(ctx)
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
	return execProv.ExecuteCommand(ctx, opts)
}

// Restart runs the restart hook for the app, writing its output to w.
func (app *App) Restart(ctx context.Context, process, versionStr string, w io.Writer) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("---- Restarting process %q ----", process)
	if process == "" {
		msg = fmt.Sprintf("---- Restarting the app %q ----", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(ctx, versionStr)
	if err != nil {
		return err
	}
	err = prov.Restart(ctx, app, process, version, w)
	if err != nil {
		log.Errorf("[restart] error on restart the app %s - %s", app.Name, err)
		return newErrorWithLog(ctx, err, app, "restart")
	}
	err = rebuild.RebuildRoutesWithAppName(app.Name, w)
	return err
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

func generateVersionProcessPastUnitsMap(version appTypes.AppVersion, units []provTypes.Unit, process string) map[vpPair]int {
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
	provisioner, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	units, err := provisioner.Units(ctx, app)
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
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(ctx, versionStr)
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

func (app *App) GetPlan() appTypes.Plan {
	return app.Plan
}

func (app *App) GetAddresses(ctx context.Context) ([]string, error) {
	routers, err := app.GetRoutersWithAddr(ctx)
	if err != nil {
		return nil, err
	}
	addresses := make([]string, len(routers))
	for i := range routers {
		addresses[i] = routers[i].Address
	}
	return addresses, nil
}

func (app *App) GetInternalBindableAddresses(ctx context.Context) ([]string, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	interAppProv, ok := prov.(provision.InterAppProvisioner)
	if !ok {
		return nil, nil
	}
	addrs, err := interAppProv.InternalAddresses(ctx, app)
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

func (app *App) GetQuotaInUse(ctx context.Context) (int, error) {
	units, err := app.Units(ctx)
	if err != nil {
		return 0, err
	}
	counter := 0
	for _, u := range units {
		switch u.Status {
		case provTypes.UnitStatusStarting, provTypes.UnitStatusStarted, provTypes.UnitStatusStopped:
			counter++
		}
	}
	return counter, nil
}

func (app *App) GetQuota(ctx context.Context) (*quota.Quota, error) {
	return servicemanager.AppQuota.Get(ctx, app)
}

func (app *App) SetQuotaLimit(ctx context.Context, limit int) error {
	return servicemanager.AppQuota.SetLimit(ctx, app, limit)
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

// Envs returns a map representing the apps environment variables.
func (app *App) Envs() map[string]bindTypes.EnvVar {
	mergedEnvs := make(map[string]bindTypes.EnvVar, len(app.Env)+len(app.ServiceEnvs)+1)
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
		envVar := e.EnvVar
		envVar.ManagedBy = fmt.Sprintf("%s/%s", e.ServiceName, e.InstanceName)
		mergedEnvs[e.Name] = envVar
	}
	sort.Strings(toInterpolateKeys)
	for _, envName := range toInterpolateKeys {
		tsuruEnvs.Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}
	mergedEnvs[tsuruEnvs.TsuruServicesEnvVar] = tsuruEnvs.ServiceEnvsFromEnvVars(app.ServiceEnvs)

	mergedEnvs["TSURU_APPNAME"] = bindTypes.EnvVar{
		Name:      "TSURU_APPNAME",
		Value:     app.Name,
		ManagedBy: "tsuru",
	}

	mergedEnvs["TSURU_APPDIR"] = bindTypes.EnvVar{
		Name:      "TSURU_APPDIR",
		Value:     defaultAppDir,
		ManagedBy: "tsuru",
	}

	return mergedEnvs
}

// SetEnvs saves a list of environment variables in the app.
func (app *App) SetEnvs(ctx context.Context, setEnvs bind.SetEnvArgs) error {
	if setEnvs.ManagedBy == "" && len(setEnvs.Envs) == 0 {
		return nil
	}

	envNames := []string{}
	for _, env := range setEnvs.Envs {
		err := validateEnv(env.Name)
		if err != nil {
			return err
		}
		envNames = append(envNames, env.Name)
	}

	if setEnvs.Writer != nil && len(setEnvs.Envs) > 0 {
		fmt.Fprintf(setEnvs.Writer, "---- Setting %d new environment variables ----\n", len(setEnvs.Envs))
	}

	err := validateEnvConflicts(app, envNames)
	if err != nil {
		fmt.Fprintf(setEnvs.Writer, "---- environment variables have conflicts with service binds: %s ----\n", err.Error())
		return err
	}

	if setEnvs.PruneUnused {
		for name, value := range app.Env {
			ok := envInSet(name, setEnvs.Envs)
			// only prune variables managed by requested
			if !ok && value.ManagedBy == setEnvs.ManagedBy {
				if setEnvs.Writer != nil {
					fmt.Fprintf(setEnvs.Writer, "---- Pruning %s from environment variables ----\n", name)
				}
				delete(app.Env, name)
			}
		}
	}

	for _, env := range setEnvs.Envs {
		app.setEnv(env)
	}

	conn, err := db.Conn()
	if err != nil {
		return err
	}

	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"env": app.Env}})
	if err != nil {
		return err
	}

	if setEnvs.ShouldRestart {
		return app.restartIfUnits(ctx, setEnvs.Writer)
	}

	return nil
}

func validateEnvConflicts(app *App, envNames []string) error {
	serviceEnvs := map[string]bindTypes.ServiceEnvVar{}
	for _, env := range app.ServiceEnvs {
		serviceEnvs[env.Name] = env
	}

	multiError := &tsuruErrors.MultiError{}

	for _, env := range envNames {
		if serviceEnv, ok := serviceEnvs[env]; ok {
			multiError.Add(fmt.Errorf("Environment variable %q is already in use by service bind \"%s/%s\"", env, serviceEnv.ServiceName, serviceEnv.InstanceName))
		}
	}

	return multiError.ToError()
}

// UnsetEnvs removes environment variables from an app, serializing the
// remaining list of environment variables to all units of the app.
func (app *App) UnsetEnvs(ctx context.Context, unsetEnvs bind.UnsetEnvArgs) error {
	if len(unsetEnvs.VariableNames) == 0 {
		return nil
	}
	if unsetEnvs.Writer != nil {
		fmt.Fprintf(unsetEnvs.Writer, "---- Unsetting %d environment variables ----\n", len(unsetEnvs.VariableNames))
	}
	for _, name := range unsetEnvs.VariableNames {
		delete(app.Env, name)
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"env": app.Env}})
	if err != nil {
		return err
	}
	if unsetEnvs.ShouldRestart {
		return app.restartIfUnits(ctx, unsetEnvs.Writer)
	}
	return nil
}

func (app *App) restartIfUnits(ctx context.Context, w io.Writer) error {
	units, err := app.Units(ctx)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
	if err != nil {
		return err
	}
	err = prov.Restart(ctx, app, "", version, w)
	if err != nil {
		return newErrorWithLog(ctx, err, app, "restart")
	}
	return nil
}

// AddCName adds a CName to app. It updates the attribute,
// calls the SetCName function on the provisioner and saves
// the app in the database, returning an error when it cannot save the change
// in the database or add the CName on the provisioner.
func (app *App) AddCName(ctx context.Context, cnames ...string) error {
	actions := []*action.Action{
		&validateNewCNames,
		&saveCNames,
		&updateApp,
	}
	err := action.NewPipeline(actions...).Execute(ctx, app, cnames)
	if err != nil {
		return err
	}
	err = rebuild.RebuildRoutesWithAppName(app.Name, nil)
	return err
}

func (app *App) RemoveCName(ctx context.Context, cnames ...string) error {
	actions := []*action.Action{
		&checkCNameExists,
		&removeCNameFromDatabase,
		&rebuildRoutes,
	}
	return action.NewPipeline(actions...).Execute(ctx, app, cnames)
}

func (app *App) AddInstance(ctx context.Context, addArgs bind.AddInstanceArgs) error {
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
		return app.restartIfUnits(ctx, addArgs.Writer)
	}
	return nil
}

func (app *App) RemoveInstance(ctx context.Context, removeArgs bind.RemoveInstanceArgs) error {
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
		return app.restartIfUnits(ctx, removeArgs.Writer)
	}
	return nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// fields in the log instance received as an example.
func (app *App) LastLogs(ctx context.Context, logService appTypes.AppLogService, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	prov, err := app.getProvisioner(ctx)
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
	args.Name = app.Name
	args.Type = "app"
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
	Units []provTypes.Unit
	Err   error
}

func Units(ctx context.Context, apps []App) (map[string]AppUnitsResponse, error) {
	poolProvMap := map[string]provision.Provisioner{}
	provMap := map[provision.Provisioner][]provision.App{}
	for i, a := range apps {
		prov, ok := poolProvMap[a.Pool]
		if !ok {
			var err error
			prov, err = a.getProvisioner(ctx)
			if err != nil {
				return nil, err
			}
			poolProvMap[a.Pool] = prov
		}
		provMap[prov] = append(provMap[prov], &apps[i])
	}
	type parallelRsp struct {
		provApps []provision.App
		units    []provTypes.Unit
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
	if filter != nil && len(filter.Statuses) > 0 {
		appsProvisionerMap := make(map[string][]provision.App)
		var prov provision.Provisioner
		for i := range apps {
			a := &apps[i]
			prov, err = a.getProvisioner(ctx)
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
			GetAppRouterUpdater().update(ctx, a)
		}
	}
	return nil
}

func (app *App) hasMultipleVersions(ctx context.Context) (bool, error) {
	prov, err := app.getProvisioner(ctx)
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

// Start starts the app calling the provisioner.Start method and
// changing the units state to StatusStarted.
func (app *App) Start(ctx context.Context, w io.Writer, process, versionStr string) error {
	w = app.withLogWriter(w)
	msg := fmt.Sprintf("\n ---> Starting the process %q", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Starting the app %q", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	version, err := app.getVersionAllowNil(ctx, versionStr)
	if err != nil {
		return err
	}
	err = prov.Start(ctx, app, process, version, w)
	if err != nil {
		log.Errorf("[start] error on start the app %s - %s", app.Name, err)
		return newErrorWithLog(ctx, err, app, "start")
	}
	err = rebuild.RebuildRoutesWithAppName(app.Name, w)
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

func (app *App) AddRouter(ctx context.Context, appRouter appTypes.AppRouter) error {
	for _, r := range app.GetRouters() {
		if appRouter.Name == r.Name {
			return ErrRouterAlreadyLinked
		}
	}
	cnames := app.GetCname()
	appCName := App{}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"cname": bson.M{"$in": cnames}, "name": bson.M{"$ne": app.Name}, "routers": appRouter}).One(&appCName)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	if appCName.Name != "" {
		for _, cname := range appCName.GetCname() {
			if cnameInSet(cname, cnames) {
				return errors.New(fmt.Sprintf("cname %s already exists for app %s using router %s", cname, appCName.Name, appRouter.Name))
			}
		}
	}
	r, err := router.Get(ctx, appRouter.Name)
	if err != nil {
		return err
	}

	// skip rebuild routes task if app has no units available
	if app.available(ctx) {
		err = rebuild.RebuildRoutesInRouter(ctx, appRouter, rebuild.RebuildRoutesOpts{
			App: app,
		})
	}

	if err != nil {
		return err
	}
	routers := append(app.GetRouters(), appRouter)
	err = app.updateRoutersDB(routers)
	if err != nil {
		rollbackErr := r.RemoveBackend(ctx, app)
		if rollbackErr != nil {
			log.Errorf("unable to remove router backend rolling back add router: %v", rollbackErr)
		}
		return err
	}
	return nil
}

func cnameInSet(cname string, cnames []string) bool {
	for _, v := range cnames {
		if v == cname {
			return true
		}
	}
	return false
}

func (app *App) UpdateRouter(ctx context.Context, appRouter appTypes.AppRouter) error {
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

	existing.Opts = appRouter.Opts
	err := app.updateRoutersDB(routers)
	if err != nil {
		return err
	}

	err = rebuild.RebuildRoutesInRouter(ctx, appRouter, rebuild.RebuildRoutesOpts{
		App: app,
	})
	if err != nil {
		return err
	}

	return nil
}

func (app *App) RemoveRouter(ctx context.Context, name string) error {
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
	r, err := router.Get(ctx, name)
	if err != nil {
		return err
	}
	err = app.updateRoutersDB(routers)
	if err != nil {
		return err
	}
	err = r.RemoveBackend(ctx, app)
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

func (app *App) GetRoutersWithAddr(ctx context.Context) ([]appTypes.AppRouter, error) {
	routers := app.GetRouters()
	multi := tsuruErrors.NewMultiError()
	for i := range routers {
		routerName := routers[i].Name
		r, planRouter, err := router.GetWithPlanRouter(ctx, routerName)
		if err != nil {
			multi.Add(err)
			continue
		}
		routers[i].Type = planRouter.Type

		addrs, aErr := r.Addresses(ctx, app)
		if aErr == nil {
			routers[i].Addresses = addrs
		} else {
			routers[i].Status = "not ready"
			if errors.Cause(aErr) != router.ErrBackendNotFound {
				routers[i].StatusDetail = aErr.Error()
			}
			multi.Add(aErr)
			continue
		}

		if len(addrs) > 0 {
			routers[i].Address = addrs[0]
			servicemanager.AppCache.Create(ctx, cache.CacheEntry{
				Key:   appRouterAddrKey(app.Name, routerName),
				Value: addrs[0],
			})
		}

		status, stErr := r.GetBackendStatus(ctx, app)
		if stErr == nil {
			routers[i].Status = string(status.Status)
			routers[i].StatusDetail = status.Detail
		} else {
			multi.Add(stErr)
		}

	}
	return routers, multi.ToError()
}

func (app *App) Shell(ctx context.Context, opts provision.ExecOptions) error {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	execProv, ok := prov.(provision.ExecutableProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "running shell"}
	}
	opts.App = app
	opts.Cmds = cmdsForExec("[ $(command -v bash) ] && exec bash -l || exec sh -l")
	return execProv.ExecuteCommand(ctx, opts)
}

func (app *App) SetCertificate(ctx context.Context, name, certificate, key string) error {
	err := app.validateNameForCert(ctx, name)
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
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			return err
		}
		tlsRouter, ok := r.(router.DefaultTLSRouter)
		if !ok {
			continue
		}
		addedAny = true
		err = tlsRouter.AddCertificate(ctx, app, name, certificate, key)
		if err != nil {
			return err
		}
	}
	if !addedAny {
		return errors.New("no router with tls support")
	}
	return nil
}

func (app *App) SetCertificateWithIssuer(ctx context.Context, name, issuer string) error {
	err := app.validateNameForCert(ctx, name)
	if err != nil {
		return err
	}

	addedAny := false
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			return err
		}
		cmRouter, ok := r.(router.CertmanagerTLSRouter)
		if !ok {
			continue
		}
		addedAny = true
		err = cmRouter.IssueCertificate(ctx, app, name, issuer)
		if err != nil {
			return err
		}
	}

	if !addedAny {
		return errors.New("no router with cert-manager support")
	}

	return nil
}

func (app *App) RemoveCertificate(ctx context.Context, name string) error {
	err := app.validateNameForCert(ctx, name)
	if err != nil {
		return err
	}
	removedAny := false
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			return err
		}
		tlsRouter, ok := r.(router.TLSRouter)
		if !ok {
			continue
		}
		removedAny = true
		err = tlsRouter.RemoveCertificate(ctx, app, name)
		if err != nil {
			return err
		}
	}
	if !removedAny {
		return errors.New("no router with tls support")
	}
	return nil
}

func (app *App) validateNameForCert(ctx context.Context, name string) error {
	addrs, err := app.GetAddresses(ctx)
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

func (app *App) GetCertificates(ctx context.Context) (map[string]map[string]string, error) {
	addrs, err := app.GetAddresses(ctx)
	if err != nil {
		return nil, err
	}
	names := append(addrs, app.CName...)
	allCertificates := make(map[string]map[string]string)
	for _, appRouter := range app.GetRouters() {
		certificates := make(map[string]string)
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			return nil, err
		}
		tlsRouter, ok := r.(router.TLSRouter)
		if !ok {
			continue
		}
		for _, n := range names {
			cert, err := tlsRouter.GetCertificate(ctx, app, n)
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
	prov, err := app.getProvisioner(ctx)
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
		evt, err = event.NewInternal(ctx, &event.Opts{
			Target:       eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
			InternalKind: "team rename",
			Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, a.Name)),
		})
		if err != nil {
			return errors.Wrap(err, "unable to create event")
		}
		defer evt.Abort(ctx)
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

func (app *App) GetHealthcheckData(ctx context.Context) (routerTypes.HealthcheckData, error) {
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
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
	prov, err := app.getProvisioner(ctx)
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
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	rprov, ok := prov.(provision.VersionsProvisioner)
	if !ok {
		return errors.Errorf("provisioner %v does not support setting versions routable", prov.GetName())
	}
	return rprov.ToggleRoutable(ctx, app, version, isRoutable)
}

func (app *App) DeployedVersions(ctx context.Context) ([]int, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	if rprov, ok := prov.(provision.VersionsProvisioner); ok {
		return rprov.DeployedVersions(ctx, app)
	}
	return nil, ErrNoVersionProvisioner
}

func (app *App) getVersion(ctx context.Context, version string) (appTypes.AppVersion, error) {
	versionProv, v, err := app.explicitVersion(ctx, version)
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
		return servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
	}
	if len(versions) > 1 {
		return nil, errors.Errorf("more than one version deployed, you must select one")
	}

	return servicemanager.AppVersion.VersionByImageOrVersion(ctx, app, strconv.Itoa(versions[0]))
}

func (app *App) getVersionAllowNil(ctx context.Context, version string) (appTypes.AppVersion, error) {
	_, v, err := app.explicitVersion(ctx, version)
	return v, err
}

func (app *App) explicitVersion(ctx context.Context, version string) (provision.VersionsProvisioner, appTypes.AppVersion, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, nil, err
	}
	versionProv, isVersionProv := prov.(provision.VersionsProvisioner)

	if !isVersionProv {
		latest, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
		if err != nil {
			return nil, nil, err
		}
		if version != "" && version != "0" {
			v, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, app, version)
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
		v, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, app, version)
		return versionProv, v, err
	}

	return versionProv, nil, nil
}

func (app *App) ListTags() []string {
	return app.Tags
}

func (app *App) GetProcess(process string) *appTypes.Process {
	for _, p := range app.Processes {
		if p.Name == process {
			return &p
		}
	}
	return nil
}

func (app *App) GetMetadata(process string) appTypes.Metadata {
	labels := map[string]string{}
	annotations := map[string]string{}

	for _, labelItem := range app.Metadata.Labels {
		labels[labelItem.Name] = labelItem.Value
	}
	for _, annotationItem := range app.Metadata.Annotations {
		annotations[annotationItem.Name] = annotationItem.Value
	}

	if process == "" {
		goto buildResult
	}

	for _, p := range app.Processes {
		if p.Name != process {
			continue
		}

		for _, labelItem := range p.Metadata.Labels {
			labels[labelItem.Name] = labelItem.Value
		}
		for _, annotationItem := range p.Metadata.Annotations {
			annotations[annotationItem.Name] = annotationItem.Value
		}
	}

buildResult:
	result := appTypes.Metadata{}

	for name, value := range labels {
		result.Labels = append(result.Labels, appTypes.MetadataItem{
			Name:  name,
			Value: value,
		})
	}

	for name, value := range annotations {
		result.Annotations = append(result.Annotations, appTypes.MetadataItem{
			Name:  name,
			Value: value,
		})
	}

	return result
}

func (app *App) AutoScaleInfo(ctx context.Context) ([]provTypes.AutoScaleSpec, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetAutoScale(ctx, app)
}

func (app *App) VerticalAutoScaleRecommendations(ctx context.Context) ([]provTypes.RecommendedResources, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetVerticalAutoScaleRecommendations(ctx, app)
}

func (app *App) UnitsMetrics(ctx context.Context) ([]provTypes.UnitMetric, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return nil, err
	}
	metricsProv, ok := prov.(provision.MetricsProvisioner)
	if !ok {
		return nil, nil
	}
	return metricsProv.UnitsMetrics(ctx, app)
}

func (app *App) AutoScale(ctx context.Context, spec provTypes.AutoScaleSpec) error {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.SetAutoScale(ctx, app, spec)
}

func (app *App) RemoveAutoScale(ctx context.Context, process string) error {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.RemoveAutoScale(ctx, app, process)
}

func envInSet(envName string, envs []bindTypes.EnvVar) bool {
	for _, e := range envs {
		if e.Name == envName {
			return true
		}
	}
	return false
}

func (app *App) GetRegistry(ctx context.Context) (imgTypes.ImageRegistry, error) {
	prov, err := app.getProvisioner(ctx)
	if err != nil {
		return "", err
	}
	registryProv, ok := prov.(provision.MultiRegistryProvisioner)
	if !ok {
		return "", nil
	}
	return registryProv.RegistryForPool(ctx, app.Pool)
}
