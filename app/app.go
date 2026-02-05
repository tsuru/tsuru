// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
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
	"text/template"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db/storagev2"
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
	"github.com/tsuru/tsuru/streamfmt"
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
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var AuthScheme auth.Scheme

var (
	ErrAlreadyHaveAccess = errors.New("team already have access to this app")
	ErrNoAccess          = errors.New("team does not have access to this app")
	ErrCannotOrphanApp   = errors.New("cannot revoke access from this team, as it's the unique team with access to the app")
	ErrDisabledPlatform  = errors.New("Disabled Platform, only admin users can create applications with the platform")

	ErrRouterAlreadyLinked = errors.New("router already linked to this app")
	ErrNoRouterWithTLS     = errors.New("no router with tls support")

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
	routerNone = "none"
)

func getBuilder(ctx context.Context, app *appTypes.App) (builder.Builder, error) {
	p, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	return builder.GetForProvisioner(p)
}

func internalAddresses(ctx context.Context, app *appTypes.App) ([]appTypes.AppInternalAddress, error) {
	provisioner, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}

	if interAppProvisioner, ok := provisioner.(provision.InterAppProvisioner); ok {
		return interAppProvisioner.InternalAddresses(ctx, app)
	}

	return nil, nil
}

func getProvisioner(ctx context.Context, app *appTypes.App) (provision.Provisioner, error) {
	return pool.GetProvisionerForPool(ctx, app.Pool)
}

// Units returns the list of units.
func AppUnits(ctx context.Context, app *appTypes.App) ([]provTypes.Unit, error) {
	prov, err := getProvisioner(ctx, app)
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
func AppInfo(ctx context.Context, app *appTypes.App) (*appTypes.AppInfo, error) {
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
		Tags:        app.Tags,
		Metadata:    app.Metadata,
	}

	if version := image.GetPlatformVersion(app); version != "latest" {
		result.Platform = fmt.Sprintf("%s:%s", app.Platform, version)
	}
	prov, err := getProvisioner(ctx, app)
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
	units, err := AppUnits(ctx, app)
	result.Units = units
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to list app units: %+v", err))
	}

	routers, err := GetRoutersWithAddr(ctx, app)
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

	q, err := GetQuota(ctx, app)
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
	autoscale, err := AutoScaleInfo(ctx, app)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale info: %+v", err))
	}
	if autoscale != nil {
		result.Autoscale = autoscale
	}
	autoscaleRec, err := VerticalAutoScaleRecommendations(ctx, app)
	if err != nil {
		errMsgs = append(errMsgs, fmt.Sprintf("unable to get autoscale recommendation info: %+v", err))
	}
	if autoscaleRec != nil {
		result.AutoscaleRecommendation = autoscaleRec
	}
	unitMetrics, err := UnitsMetrics(ctx, app)
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

	dashboardURLTemplate, _ := config.GetString("apps:dashboard-url:template")
	if dashboardURLTemplate != "" {
		tpl, tplErr := template.New("dashboardURL").Parse(dashboardURLTemplate)
		if tplErr != nil {
			return nil, fmt.Errorf("could not parse dashboard template: %w", tplErr)
		}

		var buf bytes.Buffer
		tplErr = tpl.Execute(&buf, result)
		if tplErr != nil {
			return nil, fmt.Errorf("could not execute dashboard template: %w", tplErr)
		}
		result.DashboardURL = strings.TrimSpace(buf.String())
	}

	return result, nil
}

// GetByName queries the database to find an app identified by the given
// name.
func GetByName(ctx context.Context, name string) (*appTypes.App, error) {
	var app appTypes.App
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return nil, err
	}
	err = collection.FindOne(ctx, mongoBSON.M{"name": name}).Decode(&app)
	if err == mongo.ErrNoDocuments {
		return nil, appTypes.ErrAppNotFound
	}
	if err != nil {
		return nil, err
	}

	return &app, nil
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of the following steps:
//
//  1. Save the app in the database
//  2. Provision the app using the provisioner
func CreateApp(ctx context.Context, app *appTypes.App, user *auth.User) error {
	if _, err := GetByName(ctx, app.Name); err != appTypes.ErrAppNotFound {
		if err != nil {
			return errors.WithMessage(err, "unable to check if app already exists")
		}
		return &appTypes.AppCreationError{Err: ErrAppAlreadyExists, App: app.Name}
	}
	var err error
	err = SetPool(ctx, app)
	if err != nil {
		return err
	}
	appPool, err := pool.GetPoolByName(ctx, app.Pool)
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
	err = configureCreateRouters(ctx, app)
	if err != nil {
		return err
	}
	app.Teams = []string{app.TeamOwner}
	app.Owner = user.Email
	app.Tags = processTags(app.Tags)
	if app.Platform != "" {
		app.Platform, app.PlatformVersion, err = getPlatformNameAndVersion(ctx, app, app.Platform)
		if err != nil {
			return err
		}
	}
	pruneProcesses(app)
	err = validateNew(ctx, app)
	if err != nil {
		return err
	}
	actions := []*action.Action{
		&reserveTeamApp,
		&reserveUserApp,
		&insertApp,
		&exportEnvironmentsAction,
		&provisionApp,
		&bootstrapDeployApp,
	}

	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(ctx, app, user)
	if err != nil {
		return &appTypes.AppCreationError{App: app.Name, Err: err}
	}
	return nil
}

func configureCreateRouters(ctx context.Context, app *appTypes.App) error {
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
		appPool, err = pool.GetPoolByName(ctx, app.Pool)
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
	UpdateData    *appTypes.App
	Writer        io.Writer
	ShouldRestart bool
}

// Update changes informations of the application.
func Update(ctx context.Context, app *appTypes.App, args UpdateAppArgs) (err error) {
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

	oldMetadata, err := json.Marshal(oldApp.Metadata)
	if err != nil {
		return err
	}

	if description != "" {
		app.Description = description
	}
	if poolName != "" {
		app.Pool = poolName
		_, err = getPoolForApp(ctx, app, app.Pool)
		if err != nil {
			return err
		}
	}
	newProv, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	oldProv, err := getProvisioner(ctx, &oldApp)
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
				Grant(ctx, app, team)
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

	processesHasChanged, err := updateProcesses(ctx, app, args.UpdateData.Processes)
	if err != nil {
		return err
	}

	app.Metadata.Update(args.UpdateData.Metadata)

	newMetadata, err := json.Marshal(app.Metadata)
	if err != nil {
		return err
	}

	if platform != "" {
		var p, v string
		p, v, err = getPlatformNameAndVersion(ctx, app, platform)
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
	err = validate(ctx, app)
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
	} else if string(newMetadata) != string(oldMetadata) && args.ShouldRestart {
		actions = append(actions, &restartApp)
	}
	return action.NewPipeline(actions...).Execute(ctx, app, &oldApp, args.Writer)
}

func updateProcesses(ctx context.Context, app *appTypes.App, new []appTypes.Process) (changed bool, err error) {
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

	pruneProcesses(app)

	newProcesses, err := json.Marshal(app.Processes)
	if err != nil {
		return false, errors.WithMessage(err, "could not serialize app process")
	}

	return string(oldProcesses) != string(newProcesses), nil
}

func pruneProcesses(app *appTypes.App) {
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

func validateVolumes(ctx context.Context, app *appTypes.App) error {
	volumes, err := servicemanager.Volume.ListByApp(ctx, app.Name)
	if err != nil {
		return err
	}
	if len(volumes) > 0 {
		return fmt.Errorf("can't change the provisioner of an app with binded volumes")
	}
	return nil
}

func getPlatformNameAndVersion(ctx context.Context, app *appTypes.App, platform string) (string, string, error) {
	repo, version := image.SplitImageName(platform)
	p, err := servicemanager.Platform.FindByName(ctx, repo)
	if err != nil {
		return "", "", err
	}
	reg, err := GetRegistry(ctx, app)
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
func unbind(ctx context.Context, app *appTypes.App, evt *event.Event, requestID string) error {
	instances, err := service.GetServiceInstancesBoundToApp(ctx, app.Name)
	if err != nil {
		return err
	}
	var msg string
	addMsg := func(instanceName string, reason error) {
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

func unbindVolumes(ctx context.Context, app *appTypes.App) error {
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
func Delete(ctx context.Context, app *appTypes.App, evt *event.Event, requestID string) error {
	w := evt
	appName := app.Name
	streamfmt.FprintlnSectionf(w, "Removing application %q...", appName)
	var hasErrors bool
	defer func() {
		var problems string
		if hasErrors {
			problems = " Some errors occurred during removal."
		}
		streamfmt.FprintlnSectionf(w, "Done removing application.%s", problems)
	}()
	logErr := func(msg string, err error) {
		msg = fmt.Sprintf("%s: %s", msg, err)
		fmt.Fprintf(w, "%s\n", msg)
		log.Errorf("[delete-app: %s] %s", appName, msg)
		hasErrors = true
	}
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	err = Stop(ctx, app, w, "", "")
	if err != nil {
		logErr("Unable to stop app", err)
	}
	err = unbind(ctx, app, evt, requestID)
	if err != nil {
		logErr("Unable to unbind app", err)
	}

	err = registry.RemoveAppImages(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove images from registry for app %s: %s", appName, err)
	}

	err = servicemanager.AppVersion.DeleteVersions(ctx, appName)
	if err != nil {
		log.Errorf("failed to remove image names from storage for app %s: %s", appName, err)
	}
	routers := GetRouters(app)
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
	err = unbindVolumes(ctx, app)
	if err != nil {
		logErr("Unable to unbind volumes", err)
	}
	owner, err := auth.GetUserByEmail(ctx, app.Owner)
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

	collection, err := storagev2.AppsCollection()
	if err == nil {
		_, err = collection.DeleteOne(ctx, mongoBSON.M{"name": appName})
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
func DeleteVersion(ctx context.Context, app *appTypes.App, w io.Writer, versionStr string) error {
	w = withLogWriter(app, w)
	msg := "\n" + streamfmt.Actionf("Deleting version %s of app %s", versionStr, app.Name)
	fmt.Fprintln(w, msg)
	var hasErrors bool
	defer func() {
		var problems string
		if hasErrors {
			problems = " Some errors occurred during removal."
		}
		streamfmt.FprintlnSectionf(w, "Done removing application version %s.%s", versionStr, problems)
	}()

	logErr := func(msg string, err error) {
		msg = fmt.Sprintf("%s: %s", msg, err)
		fmt.Fprintf(w, "%s\n", msg)
		log.Errorf("[delete-app-version: %s-%s] %s", app.Name, versionStr, msg)
		hasErrors = true
	}

	_, version, err := explicitVersion(ctx, app, versionStr)
	if err != nil {
		return err
	}
	prov, err := getProvisioner(ctx, app)
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
func AddUnits(ctx context.Context, app *appTypes.App, n uint, process, versionStr string, w io.Writer) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
	}

	err := ensureNoAutoscaler(ctx, app, process)
	if err != nil {
		return err
	}

	units, err := AppUnits(ctx, app)
	if err != nil {
		return err
	}
	for _, u := range units {
		if u.Status == provTypes.UnitStatusStopped {
			return errors.New("Cannot add units to an app that has stopped units")
		}
	}
	version, err := getVersion(ctx, app, versionStr)
	if err != nil {
		return err
	}
	w = withLogWriter(app, w)
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

func ensureNoAutoscaler(ctx context.Context, app *appTypes.App, process string) error {
	prov, err := getProvisioner(ctx, app)
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
func RemoveUnits(ctx context.Context, app *appTypes.App, n uint, process, versionStr string, w io.Writer) error {
	err := ensureNoAutoscaler(ctx, app, process)
	if err != nil {
		return err
	}
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	w = withLogWriter(app, w)
	version, err := getVersion(ctx, app, versionStr)
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

func KillUnit(ctx context.Context, app *appTypes.App, unitName string, force bool) error {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	unitProv, ok := prov.(provision.KillUnitProvisioner)
	if !ok {
		return ErrKillUnitProvisioner
	}
	return unitProv.KillUnit(ctx, app, unitName, force)
}

// available returns true if at least one of N units is started or unreachable.
func available(ctx context.Context, app *appTypes.App) bool {
	units, err := AppUnits(ctx, app)
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

func findTeam(app *appTypes.App, team *authTypes.Team) (int, bool) {
	for i, teamName := range app.Teams {
		if teamName == team.Name {
			return i, true
		}
	}
	return -1, false
}

// Grant allows a team to have access to an app. It returns an error if the
// team already have access to the app.
func Grant(ctx context.Context, app *appTypes.App, team *authTypes.Team) error {
	if _, found := findTeam(app, team); found {
		return ErrAlreadyHaveAccess
	}
	app.Teams = append(app.Teams, team.Name)
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$addToSet": mongoBSON.M{"teams": team.Name}})
	if err != nil {
		return err
	}

	return nil
}

// Revoke removes the access from a team. It returns an error if the team do
// not have access to the app.
func Revoke(ctx context.Context, app *appTypes.App, team *authTypes.Team) error {
	if len(app.Teams) == 1 {
		return ErrCannotOrphanApp
	}
	index, found := findTeam(app, team)
	if !found {
		return ErrNoAccess
	}
	last := len(app.Teams) - 1
	app.Teams[index] = app.Teams[last]
	app.Teams = app.Teams[:last]
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$pull": mongoBSON.M{"teams": team.Name}})
	if err != nil {
		return err
	}
	return nil
}

func SetPool(ctx context.Context, app *appTypes.App) error {
	poolName, err := getPoolForApp(ctx, app, app.Pool)
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
	return validateTeamOwner(ctx, app, p)
}

func getPoolForApp(ctx context.Context, app *appTypes.App, poolName string) (string, error) {
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
func setEnv(app *appTypes.App, env bindTypes.EnvVar) {
	if app.Env == nil {
		app.Env = make(map[string]bindTypes.EnvVar)
	}
	app.Env[env.Name] = env
	if env.Public {
		servicemanager.LogService.Add(app.Name, fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru", "api")
	}
}

// validateNew checks app name format, pool and plan
func validateNew(ctx context.Context, app *appTypes.App) error {
	if !validation.ValidateName(app.Name) {
		msg := "Invalid app name, your app should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return validate(ctx, app)
}

// validate checks app pool and plan
func validate(ctx context.Context, app *appTypes.App) error {
	err := validatePool(ctx, app)
	if err != nil {
		return err
	}

	err = validateProcesses(app)
	if err != nil {
		return err
	}

	return validatePlan(ctx, app)
}

func validatePlan(ctx context.Context, app *appTypes.App) error {
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

func validatePool(ctx context.Context, app *appTypes.App) error {
	pool, err := pool.GetPoolByName(ctx, app.Pool)
	if err != nil {
		return err
	}
	err = validateTeamOwner(ctx, app, pool)
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
		err = ValidateService(ctx, app, serviceNames...)
		if err != nil {
			return err
		}
	}

	return pool.ValidateRouters(ctx, GetRouters(app))
}

func validateProcesses(app *appTypes.App) error {
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

func validateTeamOwner(ctx context.Context, app *appTypes.App, p *pool.Pool) error {
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

func ValidateService(ctx context.Context, app *appTypes.App, services ...string) error {
	_, err := pool.GetPoolByName(ctx, app.Pool)
	if err != nil {
		return err
	}

	return pool.ValidatePoolService(ctx, app.Pool, services)
}

// InstanceEnvs returns a map of environment variables that belongs to the
// given service and service instance.
func InstanceEnvs(app *appTypes.App, serviceName, instanceName string) map[string]bindTypes.EnvVar {
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
func Run(ctx context.Context, app *appTypes.App, cmd string, w io.Writer, args provision.RunArgs) error {
	if !args.Isolated && !available(ctx, app) {
		return errors.New("App must be available to run non-isolated commands")
	}
	logWriter := LogWriter{AppName: app.Name, Source: "app-run"}
	logWriter.Async()
	defer logWriter.Close()
	logWriter.Write([]byte(fmt.Sprintf("running '%s'", cmd)))
	return run(ctx, app, cmd, io.MultiWriter(w, &logWriter), args)
}

func cmdsForExec(cmd string) []string {
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := fmt.Sprintf("[ -d %s ] && cd %s", appTypes.DefaultAppDir, appTypes.DefaultAppDir)
	return []string{"/bin/sh", "-c", fmt.Sprintf("%s; %s; %s", source, cd, cmd)}
}

func run(ctx context.Context, app *appTypes.App, cmd string, w io.Writer, args provision.RunArgs) error {
	prov, err := getProvisioner(ctx, app)
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
		Debug:  args.Debug,
	}
	units, err := AppUnits(ctx, app)
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
func Restart(ctx context.Context, app *appTypes.App, process, versionStr string, w io.Writer) error {
	w = withLogWriter(app, w)
	msg := streamfmt.Sectionf("Restarting process %q", process)
	if process == "" {
		msg = streamfmt.Sectionf("Restarting the app %q", app.Name)
	}
	fmt.Fprintf(w, "%s\n", msg)
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	version, err := getVersionAllowNil(ctx, app, versionStr)
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

func updatePastUnits(ctx context.Context, app *appTypes.App, version appTypes.AppVersion, process string) error {
	provisioner, err := getProvisioner(ctx, app)
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
		v, err := getVersion(ctx, app, versionStr)
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

func Stop(ctx context.Context, app *appTypes.App, w io.Writer, process, versionStr string) error {
	w = withLogWriter(app, w)
	msg := "\n" + streamfmt.Actionf("Stopping the process %q", process)
	if process == "" {
		msg = "\n" + streamfmt.Actionf("Stopping the app %q", app.Name)
	}
	fmt.Fprintln(w, msg)
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	version, err := getVersionAllowNil(ctx, app, versionStr)
	if err != nil {
		return err
	}

	err = updatePastUnits(ctx, app, version, process)
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

func GetAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	routers, err := GetRoutersWithAddr(ctx, app)
	if err != nil {
		return nil, err
	}
	addresses := make([]string, len(routers))
	for i := range routers {
		addresses[i] = routers[i].Address
	}
	return addresses, nil
}

func GetInternalBindableAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	prov, err := getProvisioner(ctx, app)
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

func GetQuotaInUse(ctx context.Context, app *appTypes.App) (int, error) {
	units, err := AppUnits(ctx, app)
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

func GetQuota(ctx context.Context, app *appTypes.App) (*quota.Quota, error) {
	return servicemanager.AppQuota.Get(ctx, app)
}

func SetQuotaLimit(ctx context.Context, app *appTypes.App, limit int) error {
	return servicemanager.AppQuota.SetLimit(ctx, app, limit)
}

// SetEnvs saves a list of environment variables in the app.
func SetEnvs(ctx context.Context, app *appTypes.App, setEnvs bindTypes.SetEnvArgs) error {
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
		streamfmt.FprintlnSectionf(setEnvs.Writer, "Setting %d new environment variables", len(setEnvs.Envs))
	}

	err := validateEnvConflicts(app, envNames)
	if err != nil {
		streamfmt.FprintlnSectionf(setEnvs.Writer, "environment variables have conflicts with service binds: %s", err.Error())
		return err
	}

	if setEnvs.PruneUnused {
		for name, value := range app.Env {
			ok := envInSet(name, setEnvs.Envs)
			// only prune variables managed by requested
			if !ok && value.ManagedBy == setEnvs.ManagedBy {
				if setEnvs.Writer != nil {
					streamfmt.FprintlnSectionf(setEnvs.Writer, "Pruning %s from environment variables", name)
				}
				delete(app.Env, name)
			}
		}
	}

	for _, env := range setEnvs.Envs {
		setEnv(app, env)
	}

	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$set": mongoBSON.M{"env": app.Env}})
	if err != nil {
		return err
	}

	if setEnvs.ShouldRestart {
		return restartIfUnits(ctx, app, setEnvs.Writer)
	}

	return nil
}

func validateEnvConflicts(app *appTypes.App, envNames []string) error {
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
func UnsetEnvs(ctx context.Context, app *appTypes.App, unsetEnvs bindTypes.UnsetEnvArgs) error {
	if len(unsetEnvs.VariableNames) == 0 {
		return nil
	}
	if unsetEnvs.Writer != nil {
		streamfmt.FprintlnSectionf(unsetEnvs.Writer, "Unsetting %d environment variables", len(unsetEnvs.VariableNames))
	}
	for _, name := range unsetEnvs.VariableNames {
		delete(app.Env, name)
	}
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$set": mongoBSON.M{"env": app.Env}})
	if err != nil {
		return err
	}
	if unsetEnvs.ShouldRestart {
		return restartIfUnits(ctx, app, unsetEnvs.Writer)
	}
	return nil
}

func restartIfUnits(ctx context.Context, app *appTypes.App, w io.Writer) error {
	units, err := AppUnits(ctx, app)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	err = prov.Restart(ctx, app, "", nil, w)
	if err != nil {
		return newErrorWithLog(ctx, err, app, "restart")
	}
	return nil
}

// AddCName adds a CName to app. It updates the attribute,
// calls the SetCName function on the provisioner and saves
// the app in the database, returning an error when it cannot save the change
// in the database or add the CName on the provisioner.
func AddCName(ctx context.Context, app *appTypes.App, cnames ...string) error {
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

func RemoveCName(ctx context.Context, app *appTypes.App, cnames ...string) error {
	actions := []*action.Action{
		&checkCNameExists,
		&removeCNameFromDatabase,
		&removeCertIssuersFromDatabase,
		&rebuildRoutes,
	}
	return action.NewPipeline(actions...).Execute(ctx, app, cnames)
}

func SetCertIssuer(ctx context.Context, app *appTypes.App, cname, certIssuer string) error {
	actions := []*action.Action{
		&checkSingleCNameExists,
		&checkCertIssuerPoolConstraints,
		&saveCertIssuer,
		&rebuildRoutes,
	}
	return action.NewPipeline(actions...).Execute(ctx, app, cname, certIssuer)
}

func UnsetCertIssuer(ctx context.Context, app *appTypes.App, cname string) error {
	actions := []*action.Action{
		&checkSingleCNameExists,
		&removeCertIssuer,
		&rebuildRoutes,
	}
	return action.NewPipeline(actions...).Execute(ctx, app, cname)
}

func AddInstance(ctx context.Context, app *appTypes.App, addArgs bindTypes.AddInstanceArgs) error {
	if len(addArgs.Envs) == 0 {
		return nil
	}
	if addArgs.Writer != nil {
		streamfmt.FprintlnSectionf(addArgs.Writer, "Setting %d new environment variables", len(addArgs.Envs)+1)
	}
	app.ServiceEnvs = append(app.ServiceEnvs, addArgs.Envs...)
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$set": mongoBSON.M{"serviceenvs": app.ServiceEnvs}})
	if err != nil {
		return err
	}
	if addArgs.ShouldRestart {
		return restartIfUnits(ctx, app, addArgs.Writer)
	}
	return nil
}

func RemoveInstance(ctx context.Context, app *appTypes.App, removeArgs bindTypes.RemoveInstanceArgs) error {
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
		streamfmt.FprintlnSectionf(removeArgs.Writer, "Unsetting %d environment variables", toUnset)
	}
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$set": mongoBSON.M{"serviceenvs": app.ServiceEnvs}})
	if err != nil {
		return err
	}
	if removeArgs.ShouldRestart {
		return restartIfUnits(ctx, app, removeArgs.Writer)
	}
	return nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// fields in the log instance received as an example.
func LastLogs(ctx context.Context, app *appTypes.App, logService appTypes.AppLogService, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	prov, err := getProvisioner(ctx, app)
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

func (f *Filter) Query() mongoBSON.M {
	if f == nil {
		return mongoBSON.M{}
	}
	query := mongoBSON.M{}
	if f.Extra != nil {
		var orBlock []mongoBSON.M
		for field, values := range f.Extra {
			orBlock = append(orBlock, mongoBSON.M{
				field: mongoBSON.M{"$in": values},
			})
		}
		query["$or"] = orBlock
	}
	if f.NameMatches != "" {
		query["name"] = mongoBSON.M{"$regex": f.NameMatches}
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
				query["$and"] = []mongoBSON.M{
					{"$or": []mongoBSON.M{
						{"platformversion": mongoBSON.M{"$in": []string{"latest", ""}}},
						{"platformversion": mongoBSON.M{"$exists": false}},
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
	if len(f.Pools) > 0 {
		query["pool"] = mongoBSON.M{"$in": f.Pools}
	}
	tags := processTags(f.Tags)
	if len(tags) > 0 {
		query["tags"] = mongoBSON.M{"$all": tags}
	}
	return query
}

type AppUnitsResponse struct {
	Units []provTypes.Unit
	Err   error
}

func Units(ctx context.Context, apps []*appTypes.App) (map[string]AppUnitsResponse, error) {
	poolProvMap := map[string]provision.Provisioner{}
	provMap := map[provision.Provisioner][]*appTypes.App{}
	for i, a := range apps {
		prov, ok := poolProvMap[a.Pool]
		if !ok {
			var err error
			prov, err = getProvisioner(ctx, a)
			if err != nil {
				return nil, err
			}
			poolProvMap[a.Pool] = prov
		}
		provMap[prov] = append(provMap[prov], apps[i])
	}
	type parallelRsp struct {
		provApps []*appTypes.App
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
				rsp := appUnits[a.Name]
				rsp.Err = errors.Wrap(pRsp.err, "unable to list app units")
				appUnits[a.Name] = rsp
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
func List(ctx context.Context, filter *Filter) ([]*appTypes.App, error) {
	apps := []*appTypes.App{}
	query := filter.Query()
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return nil, err
	}

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &apps)
	if err != nil {
		return nil, err
	}

	if filter != nil && len(filter.Statuses) > 0 {
		appsProvisionerMap := make(map[string][]*appTypes.App)
		var prov provision.Provisioner
		for i := range apps {
			a := apps[i]
			prov, err = getProvisioner(ctx, a)
			if err != nil {
				return nil, err
			}
			appsProvisionerMap[prov.GetName()] = append(appsProvisionerMap[prov.GetName()], a)
		}
		var provisionApps []*appTypes.App
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
		copy(apps, provisionApps)
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

func loadCachedAddrsInApps(ctx context.Context, apps []*appTypes.App) error {
	keys := make([]string, 0, len(apps))
	for i := range apps {
		a := apps[i]
		a.Routers = GetRouters(a)
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
		a := apps[i]
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

func hasMultipleVersions(ctx context.Context, app *appTypes.App) (bool, error) {
	prov, err := getProvisioner(ctx, app)
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
func Start(ctx context.Context, app *appTypes.App, w io.Writer, process, versionStr string) error {
	w = withLogWriter(app, w)
	msg := "\n" + streamfmt.Actionf("Starting the process %q", process)
	if process == "" {
		msg = "\n" + streamfmt.Actionf("Starting the app %q", app.Name)
	}
	fmt.Fprintln(w, msg)
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	version, err := getVersionAllowNil(ctx, app, versionStr)
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

func SetUpdatePlatform(ctx context.Context, app *appTypes.App, check bool) error {
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(
		ctx,
		mongoBSON.M{"name": app.Name},
		mongoBSON.M{"$set": mongoBSON.M{"updateplatform": check}},
	)

	return err
}

func AddRouter(ctx context.Context, app *appTypes.App, appRouter appTypes.AppRouter) error {
	for _, r := range GetRouters(app) {
		if appRouter.Name == r.Name {
			return ErrRouterAlreadyLinked
		}
	}
	cnames := app.CName
	appCName := &appTypes.App{}
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	err = collection.FindOne(ctx, mongoBSON.M{"cname": mongoBSON.M{"$in": cnames}, "name": mongoBSON.M{"$ne": app.Name}, "routers": appRouter}).Decode(&appCName)
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	if appCName.Name != "" {
		for _, cname := range appCName.CName {
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
	if available(ctx, app) {
		err = rebuild.RebuildRoutesInRouter(ctx, appRouter, rebuild.RebuildRoutesOpts{
			App: app,
		})
	}

	if err != nil {
		return err
	}
	routers := append(GetRouters(app), appRouter)
	err = updateRoutersDB(ctx, app, routers)
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

func UpdateRouter(ctx context.Context, app *appTypes.App, appRouter appTypes.AppRouter) error {
	var existing *appTypes.AppRouter
	routers := GetRouters(app)
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
	err := updateRoutersDB(ctx, app, routers)
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

func RemoveRouter(ctx context.Context, app *appTypes.App, name string) error {
	removed := false
	routers := GetRouters(app)
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
	err = updateRoutersDB(ctx, app, routers)
	if err != nil {
		return err
	}
	err = r.RemoveBackend(ctx, app)
	if err != nil {
		log.Errorf("unable to remove router backend: %v", err)
	}
	return nil
}

func updateRoutersDB(ctx context.Context, app *appTypes.App, routers []appTypes.AppRouter) error {
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	app.Routers = routers
	app.Router = ""
	app.RouterOpts = nil
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{
		"$set": mongoBSON.M{
			"routers":    app.Routers,
			"router":     app.Router,
			"routeropts": app.RouterOpts,
		},
	})

	return err
}

func GetRouters(app *appTypes.App) []appTypes.AppRouter {
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

func GetRoutersWithAddr(ctx context.Context, app *appTypes.App) ([]appTypes.AppRouter, error) {
	routers := GetRouters(app)
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

func Shell(ctx context.Context, app *appTypes.App, opts provision.ExecOptions) error {
	prov, err := getProvisioner(ctx, app)
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

func SetCertificate(ctx context.Context, app *appTypes.App, name, certificate, key string) error {
	err := validateNameForCert(ctx, app, name)
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
	for _, appRouter := range GetRouters(app) {
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			return err
		}
		tlsRouter, ok := r.(router.TLSRouter)
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
		return ErrNoRouterWithTLS
	}
	return nil
}

func RemoveCertificate(ctx context.Context, app *appTypes.App, name string) error {
	err := validateNameForCert(ctx, app, name)
	if err != nil {
		return err
	}
	removedAny := false
	for _, appRouter := range GetRouters(app) {
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
		return ErrNoRouterWithTLS
	}
	return nil
}

func validateNameForCert(ctx context.Context, app *appTypes.App, name string) error {
	addrs, err := GetAddresses(ctx, app)
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

func GetCertificates(ctx context.Context, app *appTypes.App) (*appTypes.CertificateSetInfo, error) {
	addrs, err := GetAddresses(ctx, app)
	if err != nil {
		return nil, err
	}

	certificateSet := &appTypes.CertificateSetInfo{
		Routers: make(map[string]appTypes.RouterCertificateInfo),
	}

	names := append([]string{}, app.CName...)

	for _, addr := range addrs {
		parsedURL, _ := url.Parse(addr)
		if parsedURL != nil {
			names = append(names, parsedURL.Hostname())
		}
	}
	for _, appRouter := range GetRouters(app) {
		appRouterCertificates := appTypes.RouterCertificateInfo{
			CNames: make(map[string]appTypes.CertificateInfo),
		}
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

			certInfo := appTypes.CertificateInfo{
				Certificate: cert,
				Issuer:      app.CertIssuers[n],
			}

			if certInfo != (appTypes.CertificateInfo{}) {
				appRouterCertificates.CNames[n] = certInfo
			}
		}

		if !appRouterCertificates.IsEmpty() {
			certificateSet.Routers[appRouter.Name] = appRouterCertificates
		}
	}

	if certificateSet.IsEmpty() {
		return nil, ErrNoRouterWithTLS
	}

	return certificateSet, nil
}

func withLogWriter(app *appTypes.App, w io.Writer) io.Writer {
	logWriter := &LogWriter{AppName: app.Name}
	if w != nil {
		w = io.MultiWriter(w, logWriter)
	} else {
		w = logWriter
	}
	return w
}

func RenameTeam(ctx context.Context, oldName, newName string) error {
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

	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}

	updates := []mongo.WriteModel{
		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teamowner": oldName}).
			SetUpdate(mongoBSON.M{"$set": mongoBSON.M{"teamowner": newName}}),

		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teams": oldName}).
			SetUpdate(mongoBSON.M{"$push": mongoBSON.M{"teams": newName}}),

		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teams": oldName}).
			SetUpdate(mongoBSON.M{"$pull": mongoBSON.M{"teams": oldName}}),
	}

	_, err = collection.BulkWrite(ctx, updates)
	return err
}

func GetHealthcheckData(ctx context.Context, app *appTypes.App) (routerTypes.HealthcheckData, error) {
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

	prov, err := pool.GetProvisionerForPool(ctx, app.Pool)
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

func SetRoutable(ctx context.Context, app *appTypes.App, version appTypes.AppVersion, isRoutable bool) error {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	rprov, ok := prov.(provision.VersionsProvisioner)
	if !ok {
		return errors.Errorf("provisioner %v does not support setting versions routable", prov.GetName())
	}
	return rprov.ToggleRoutable(ctx, app, version, isRoutable)
}

func DeployedVersions(ctx context.Context, app *appTypes.App) ([]int, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	if rprov, ok := prov.(provision.VersionsProvisioner); ok {
		return rprov.DeployedVersions(ctx, app)
	}
	return nil, ErrNoVersionProvisioner
}

func getVersion(ctx context.Context, app *appTypes.App, version string) (appTypes.AppVersion, error) {
	versionProv, v, err := explicitVersion(ctx, app, version)
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

func getVersionAllowNil(ctx context.Context, app *appTypes.App, version string) (appTypes.AppVersion, error) {
	if version == "" {
		return nil, nil
	}
	_, v, err := explicitVersion(ctx, app, version)
	return v, err
}

func explicitVersion(ctx context.Context, app *appTypes.App, version string) (provision.VersionsProvisioner, appTypes.AppVersion, error) {
	prov, err := getProvisioner(ctx, app)
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

func AutoScaleInfo(ctx context.Context, app *appTypes.App) ([]provTypes.AutoScaleSpec, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetAutoScale(ctx, app)
}

func VerticalAutoScaleRecommendations(ctx context.Context, app *appTypes.App) ([]provTypes.RecommendedResources, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return nil, nil
	}
	return autoscaleProv.GetVerticalAutoScaleRecommendations(ctx, app)
}

func UnitsMetrics(ctx context.Context, app *appTypes.App) ([]provTypes.UnitMetric, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	metricsProv, ok := prov.(provision.MetricsProvisioner)
	if !ok {
		return nil, nil
	}
	return metricsProv.UnitsMetrics(ctx, app)
}

func AutoScale(ctx context.Context, app *appTypes.App, spec provTypes.AutoScaleSpec) error {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.SetAutoScale(ctx, app, spec)
}

func RemoveAutoScale(ctx context.Context, app *appTypes.App, process string) error {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}
	return autoscaleProv.RemoveAutoScale(ctx, app, process)
}

func SwapAutoScale(ctx context.Context, app *appTypes.App, versionStr string) error {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return err
	}
	autoscaleProv, ok := prov.(provision.AutoScaleProvisioner)
	if !ok {
		return errors.Errorf("provisioner %q does not support native autoscaling", prov.GetName())
	}

	return autoscaleProv.SwapAutoScale(ctx, app, versionStr)
}

func envInSet(envName string, envs []bindTypes.EnvVar) bool {
	for _, e := range envs {
		if e.Name == envName {
			return true
		}
	}
	return false
}

func GetRegistry(ctx context.Context, app *appTypes.App) (imgTypes.ImageRegistry, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return "", err
	}
	registryProv, ok := prov.(provision.MultiRegistryProvisioner)
	if !ok {
		return "", nil
	}
	return registryProv.RegistryForPool(ctx, app.Pool)
}
