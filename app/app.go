// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
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
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var AuthScheme auth.Scheme

var (
	ErrAlreadyHaveAccess = errors.New("team already have access to this app")
	ErrNoAccess          = errors.New("team does not have access to this app")
	ErrCannotOrphanApp   = errors.New("cannot revoke access from this team, as it's the unique team with access to the app")
	ErrDisabledPlatform  = errors.New("Disabled Platform, only admin users can create applications with the platform")
)

const (
	// InternalAppName is a reserved name used for token generation. For
	// backward compatibility and historical purpose, the value remained
	// "tsr" when the name of the daemon changed to "tsurud".
	InternalAppName = "tsr"

	TsuruServicesEnvVar = "TSURU_SERVICES"
	defaultAppDir       = "/home/application/current"
)

// AppLock stores information about a lock hold on the app
type AppLock struct {
	Locked      bool
	Reason      string
	Owner       string
	AcquireDate time.Time
}

func (l *AppLock) String() string {
	if !l.Locked {
		return "Not locked"
	}
	return fmt.Sprintf("App locked by %s, running %s. Acquired in %s",
		l.Owner,
		l.Reason,
		l.AcquireDate.Format(time.RFC3339),
	)
}

func (l *AppLock) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Locked      bool   `json:"Locked"`
		Reason      string `json:"Reason"`
		Owner       string `json:"Owner"`
		AcquireDate string `json:"AcquireDate"`
	}{
		Locked:      l.Locked,
		Reason:      l.Reason,
		Owner:       l.Owner,
		AcquireDate: l.AcquireDate.Format(time.RFC3339),
	})
}

func (l *AppLock) GetLocked() bool {
	return l.Locked
}

func (l *AppLock) GetReason() string {
	return l.Reason
}

func (l *AppLock) GetOwner() string {
	return l.Owner
}

func (l *AppLock) GetAcquireDate() time.Time {
	return l.AcquireDate
}

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App struct {
	Env            map[string]bind.EnvVar
	ServiceEnvs    []bind.ServiceEnvVar
	Platform       string `bson:"framework"`
	Name           string
	IP             string
	CName          []string
	Teams          []string
	TeamOwner      string
	Owner          string
	Plan           appTypes.Plan
	UpdatePlatform bool
	Lock           AppLock
	Pool           string
	Description    string
	Router         string
	RouterOpts     map[string]string
	Deploys        uint
	Tags           []string
	Error          string

	quota.Quota
	builder     builder.Builder
	provisioner provision.Provisioner
}

var (
	_ provision.App      = &App{}
	_ rebuild.RebuildApp = &App{}
)

func (app *App) getBuilder() (builder.Builder, error) {
	if app.builder == nil {
		if app.Pool == "" {
			return builder.GetDefault()
		}
		pool, err := pool.GetPoolByName(app.Pool)
		if err != nil {
			return nil, err
		}
		if pool.Builder == "" {
			return builder.GetDefault()
		}
		app.builder, err = builder.Get(pool.Builder)
		if err != nil {
			return nil, err
		}
	}
	return app.builder, nil
}

func (app *App) getProvisioner() (provision.Provisioner, error) {
	if app.provisioner == nil {
		if app.Pool == "" {
			return provision.GetDefault()
		}
		p, err := pool.GetPoolByName(app.Pool)
		if err != nil {
			return nil, err
		}
		app.provisioner, err = p.GetProvisioner()
		if err != nil {
			return nil, err
		}
	}
	return app.provisioner, nil
}

// Units returns the list of units.
func (app *App) Units() ([]provision.Unit, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return []provision.Unit{}, err
	}
	units, err := prov.Units(app)
	if units == nil {
		// This is unusual but was done because previously this method didn't
		// return an error. This ensures we always return an empty list instead
		// of nil to preserve compatibility with old clients.
		units = []provision.Unit{}
	}
	return units, err
}

func (app *App) GetRouterOpts() map[string]string {
	return app.RouterOpts
}

// MarshalJSON marshals the app in json format.
func (app *App) MarshalJSON() ([]byte, error) {
	repo, _ := repository.Manager().GetRepository(app.Name)
	result := make(map[string]interface{})
	result["name"] = app.Name
	result["platform"] = app.Platform
	result["teams"] = app.Teams
	units, err := app.Units()
	result["units"] = units
	if err != nil {
		result["error"] = fmt.Sprintf("unable to list app units: %+v", err)
	}
	result["repository"] = repo.ReadWriteURL
	result["ip"] = app.IP
	result["cname"] = app.CName
	result["owner"] = app.Owner
	result["pool"] = app.Pool
	result["description"] = app.Description
	result["deploys"] = app.Deploys
	result["teamowner"] = app.TeamOwner
	result["plan"] = map[string]interface{}{
		"name":     app.Plan.Name,
		"memory":   app.Plan.Memory,
		"swap":     app.Plan.Swap,
		"cpushare": app.Plan.CpuShare,
		"router":   app.Router,
	}
	result["router"] = app.Router
	result["routeropts"] = app.RouterOpts
	result["lock"] = app.Lock
	result["tags"] = app.Tags
	return json.Marshal(&result)
}

// Applog represents a log entry.
type Applog struct {
	MongoID bson.ObjectId `bson:"_id,omitempty" json:"-"`
	Date    time.Time
	Message string
	Source  string
	AppName string
	Unit    string
}

type ErrAppNotLocked struct {
	App string
}

func (e ErrAppNotLocked) Error() string {
	return fmt.Sprintf("unable to lock app %q", e.App)
}

// AcquireApplicationLock acquires an application lock by setting the lock
// field in the database.  This method is already called by a connection
// middleware on requests with :app or :appname params that have side-effects.
func AcquireApplicationLock(appName string, owner string, reason string) (bool, error) {
	return AcquireApplicationLockWait(appName, owner, reason, 0)
}

// Same as AcquireApplicationLock but it keeps trying to acquire the lock
// until timeout is reached.
func AcquireApplicationLockWait(appName string, owner string, reason string, timeout time.Duration) (bool, error) {
	timeoutChan := time.After(timeout)
	conn, err := db.Conn()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	for {
		appLock := AppLock{
			Locked:      true,
			Reason:      reason,
			Owner:       owner,
			AcquireDate: time.Now().In(time.UTC),
		}
		err = conn.Apps().Update(bson.M{"name": appName, "lock.locked": bson.M{"$in": []interface{}{false, nil}}}, bson.M{"$set": bson.M{"lock": appLock}})
		if err == nil {
			return true, nil
		}
		if err != mgo.ErrNotFound {
			return false, err
		}
		select {
		case <-timeoutChan:
			return false, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func AcquireApplicationLockWaitMany(appNames []string, owner string, reason string, timeout time.Duration) error {
	lockedApps := make(chan string, len(appNames))
	errCh := make(chan error, len(appNames))
	wg := sync.WaitGroup{}
	for _, appName := range appNames {
		wg.Add(1)
		go func(appName string) {
			defer wg.Done()
			locked, err := AcquireApplicationLockWait(appName, owner, reason, timeout)
			if err != nil {
				errCh <- err
				return
			}
			if !locked {
				errCh <- ErrAppNotLocked{App: appName}
				return
			}
			lockedApps <- appName
		}(appName)
	}
	wg.Wait()
	close(lockedApps)
	close(errCh)
	err := <-errCh
	if err != nil {
		for appName := range lockedApps {
			ReleaseApplicationLock(appName)
		}
		return err
	}
	return nil
}

func ReleaseApplicationLockMany(appNames []string) {
	for _, appName := range appNames {
		ReleaseApplicationLock(appName)
	}
}

// ReleaseApplicationLock releases a lock hold on an app, currently it's called
// by a middleware, however, ideally, it should be called individually by each
// handler since they might be doing operations in background.
func ReleaseApplicationLock(appName string) {
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Error getting DB, couldn't unlock %s: %s", appName, err)
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": appName, "lock.locked": true}, bson.M{"$set": bson.M{"lock": AppLock{}}})
	if err != nil {
		log.Errorf("Error updating entry, couldn't unlock %s: %s", appName, err)
	}
}

// GetByName queries the database to find an app identified by the given
// name.
func GetByName(name string) (*App, error) {
	var app App
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Apps().Find(bson.M{"name": name}).One(&app)
	if err == mgo.ErrNotFound {
		return nil, ErrAppNotFound
	}
	return &app, err
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of the following steps:
//
//       1. Save the app in the database
//       2. Create the git repository using the repository manager
//       3. Provision the app using the provisioner
func CreateApp(app *App, user *auth.User) error {
	var plan *appTypes.Plan
	var err error
	if app.Plan.Name == "" {
		plan, err = DefaultPlan()
	} else {
		plan, err = findPlanByName(app.Plan.Name)
	}
	if err != nil {
		return err
	}
	app.Plan = *plan
	err = app.SetPool()
	if err != nil {
		return err
	}
	if app.Router == "" {
		pool, errPool := pool.GetPoolByName(app.GetPool())
		if errPool != nil {
			return errPool
		}
		app.Router, err = pool.GetDefaultRouter()
	} else {
		_, err = router.Get(app.Router)
	}
	if err != nil {
		return err
	}
	app.Teams = []string{app.TeamOwner}
	app.Owner = user.Email
	app.Tags = processTags(app.Tags)
	err = app.validate()
	if err != nil {
		return err
	}
	actions := []*action.Action{
		&reserveUserApp,
		&insertApp,
		&exportEnvironmentsAction,
		&createRepository,
		&addRouterBackend,
		&provisionApp,
		&setAppIp,
	}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, user)
	if err != nil {
		return &AppCreationError{app: app.Name, Err: err}
	}
	return nil
}

// Update changes informations of the application.
func (app *App) Update(updateData App, w io.Writer) (err error) {
	description := updateData.Description
	planName := updateData.Plan.Name
	poolName := updateData.Pool
	teamOwner := updateData.TeamOwner
	routerName := updateData.Router
	platform := updateData.Platform
	tags := processTags(updateData.Tags)
	if description != "" {
		app.Description = description
	}
	if poolName != "" {
		app.Pool = poolName
		_, err = app.getPoolForApp(app.Pool)
		if err != nil {
			return err
		}
	}
	oldPlan := app.Plan
	oldRouter := app.Router
	if routerName != "" {
		_, err = router.Get(routerName)
		if err != nil {
			return err
		}
		app.Router = routerName
	}
	oldRouterOpts := app.RouterOpts
	if len(updateData.RouterOpts) > 0 {
		app.RouterOpts = updateData.RouterOpts
	}
	if planName != "" {
		plan, errFind := findPlanByName(planName)
		if errFind != nil {
			return errFind
		}
		app.Plan = *plan
	}
	if teamOwner != "" {
		team, errTeam := auth.GetTeam(teamOwner)
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
	if platform != "" {
		p, errPlat := GetPlatform(platform)
		if errPlat != nil {
			return errPlat
		}
		if app.Platform != p.Name {
			app.UpdatePlatform = true
		}
		app.Platform = p.Name
	}
	if updateData.UpdatePlatform {
		app.UpdatePlatform = true
	}
	err = app.validate()
	if err != nil {
		return err
	}
	if app.Router != oldRouter || app.Plan != oldPlan || len(updateData.RouterOpts) > 0 {
		actions := []*action.Action{
			&moveRouterUnits,
			&updateRouterOpts,
			&saveApp,
			&restartApp,
			&removeOldBackend,
		}
		err = action.NewPipeline(actions...).Execute(app, &oldPlan, oldRouter, oldRouterOpts, w)
		if err != nil {
			return err
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(bson.M{"name": app.Name}, app)
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
func (app *App) unbind() error {
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
		err = instance.UnbindApp(app, true, nil)
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
	volumes, err := volume.ListByApp(app.Name)
	if err != nil {
		return errors.Wrap(err, "Unable to list volumes for unbind")
	}
	for _, v := range volumes {
		var binds []volume.VolumeBind
		binds, err = v.LoadBinds()
		if err != nil {
			return errors.Wrap(err, "Unable to list volume binds for unbind")
		}
		for _, b := range binds {
			err = v.UnbindApp(app.Name, b.ID.MountPoint)
			if err != nil {
				return errors.Wrapf(err, "Unable to unbind volume %q in %q", app.Name, b.ID.MountPoint)
			}
		}
	}
	return nil
}

// Delete deletes an app.
func Delete(app *App, w io.Writer) error {
	isSwapped, swappedWith, err := router.IsSwapped(app.GetName())
	if err != nil {
		return errors.Wrap(err, "unable to check if app is swapped")
	}
	if isSwapped {
		return errors.Errorf("application is swapped with %q, cannot remove it", swappedWith)
	}
	appName := app.Name
	if w == nil {
		w = ioutil.Discard
	}
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
	err = prov.Destroy(app)
	if err != nil {
		logErr("Unable to destroy app in provisioner", err)
	}
	err = image.DeleteAllAppImageNames(appName)
	if err != nil {
		log.Errorf("failed to remove image names from storage for app %s: %s", appName, err)
	}
	r, err := app.GetRouter()
	if err == nil {
		err = r.RemoveBackend(app.Name)
	}
	if err != nil {
		logErr("Failed to remove router backend", err)
	}
	err = router.Remove(app.Name)
	if err != nil {
		logErr("Failed to remove router backend from database", err)
	}
	err = app.unbind()
	if err != nil {
		logErr("Unable to unbind app", err)
	}
	err = app.unbindVolumes()
	if err != nil {
		logErr("Unable to unbind volumes", err)
	}
	err = repository.Manager().RemoveRepository(appName)
	if err != nil {
		logErr("Unable to remove app from repository manager", err)
	}
	token := app.Env["TSURU_APP_TOKEN"].Value
	err = AuthScheme.AppLogout(token)
	if err != nil {
		logErr("Unable to remove app token in destroy", err)
	}
	owner, err := auth.GetUserByEmail(app.Owner)
	if err == nil {
		err = auth.ReleaseApp(owner)
	}
	if err != nil {
		logErr("Unable to release app quota", err)
	}
	logConn, err := db.LogConn()
	if err == nil {
		defer logConn.Close()
		err = logConn.Logs(appName).DropCollection()
	}
	if err != nil {
		logErr("Unable to remove logs collection", err)
	}
	conn, err := db.Conn()
	if err == nil {
		defer conn.Close()
		err = conn.Apps().Remove(bson.M{"name": appName})
	}
	if err != nil {
		logErr("Unable to remove app from db", err)
	}
	err = event.MarkAsRemoved(event.Target{Type: event.TargetTypeApp, Value: appName})
	if err != nil {
		logErr("Unable to mark old events as removed", err)
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
func (app *App) AddUnits(n uint, process string, w io.Writer) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
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
	w = app.withLogWriter(w)
	err = action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
	).Execute(app, n, w, process)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	return err
}

// RemoveUnits removes n units from the app. It's a process composed of
// multiple steps:
//
//     1. Remove units from the provisioner
//     2. Update quota
func (app *App) RemoveUnits(n uint, process string, w io.Writer) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	w = app.withLogWriter(w)
	err = prov.RemoveUnits(app, n, process, w)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	if err != nil {
		return err
	}
	units, err := app.Units()
	if err != nil {
		return err
	}
	return app.SetQuotaInUse(len(units))
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

// UpdateNodeStatus updates the status of the given node and its units,
// returning a map which units were found during the update.
func UpdateNodeStatus(nodeData provision.NodeStatusData) ([]UpdateUnitsResult, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	var node provision.Node
	for _, p := range provisioners {
		if nodeProv, ok := p.(provision.NodeProvisioner); ok {
			node, err = nodeProv.NodeForNodeData(nodeData)
			if err == nil {
				break
			}
			if errors.Cause(err) != provision.ErrNodeNotFound {
				return nil, err
			}
		}
	}
	if node == nil {
		return nil, provision.ErrNodeNotFound
	}
	if healer.HealerInstance != nil {
		err = healer.HealerInstance.UpdateNodeData(node, nodeData.Checks)
		if err != nil {
			log.Errorf("unable to set node status in healer: %s", err)
		}
	}
	unitProv, ok := node.Provisioner().(provision.UnitStatusProvisioner)
	if !ok {
		return []UpdateUnitsResult{}, nil
	}
	result := make([]UpdateUnitsResult, len(nodeData.Units))
	for i, unitData := range nodeData.Units {
		unit := provision.Unit{ID: unitData.ID, Name: unitData.Name}
		err = unitProv.SetUnitStatus(unit, unitData.Status)
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
	users, err := auth.ListUsersWithPermissions(permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, team.Name),
	})
	if err != nil {
		conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$pull": bson.M{"teams": team.Name}})
		return err
	}
	for _, user := range users {
		err = repository.Manager().GrantAccess(app.Name, user.Email)
		if err != nil {
			conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$pull": bson.M{"teams": team.Name}})
			return err
		}
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
	users, err := auth.ListUsersWithPermissions(permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, team.Name),
	})
	if err != nil {
		conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$addToSet": bson.M{"teams": team.Name}})
		return err
	}
	for _, user := range users {
		perms, err := user.Permissions()
		if err != nil {
			conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$addToSet": bson.M{"teams": team.Name}})
			return err
		}
		canDeploy := permission.CheckFromPermList(perms, permission.PermAppDeploy,
			append(permission.Contexts(permission.CtxTeam, app.Teams),
				permission.Context(permission.CtxApp, app.Name),
				permission.Context(permission.CtxPool, app.Pool),
			)...,
		)
		if canDeploy {
			continue
		}
		err = repository.Manager().RevokeAccess(app.Name, user.Email)
		if err != nil {
			conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$addToSet": bson.M{"teams": team.Name}})
			return err
		}
	}
	return nil
}

// GetTeams returns a slice of teams that have access to the app.
func (app *App) GetTeams() []authTypes.Team {
	t, _ := auth.TeamService().FindByNames(app.Teams)
	return t
}

func (app *App) SetPool() error {
	poolName, err := app.getPoolForApp(app.Pool)
	if err != nil {
		return err
	}
	if poolName == "" {
		var p *pool.Pool
		p, err = pool.GetDefaultPool()
		if err != nil {
			return err
		}
		poolName = p.Name
	}
	app.Pool = poolName
	p, err := pool.GetPoolByName(poolName)
	if err != nil {
		return err
	}
	return app.validateTeamOwner(p)
}

func (app *App) getPoolForApp(poolName string) (string, error) {
	if poolName == "" {
		pools, err := pool.ListPoolsForTeam(app.TeamOwner)
		if err != nil {
			return "", err
		}
		if len(pools) > 1 {
			var names []string
			for _, p := range pools {
				names = append(names, fmt.Sprintf("%q", p.Name))
			}
			return "", errors.Errorf("you have access to %s pools. Please choose one in app creation", strings.Join(names, ","))
		}
		if len(pools) == 0 {
			return "", nil
		}
		return pools[0].Name, nil
	}
	pool, err := pool.GetPoolByName(poolName)
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
		app.Log(fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru", "api")
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

// validate checks app name format
func (app *App) validate() error {
	if app.Name == InternalAppName || !validation.ValidateName(app.Name) {
		msg := "Invalid app name, your app should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return app.validatePool()
}

func (app *App) validatePool() error {
	pool, err := pool.GetPoolByName(app.Pool)
	if err != nil {
		return err
	}
	err = app.validateTeamOwner(pool)
	if err != nil {
		return err
	}
	return app.validateRouter(pool)
}

func (app *App) validateTeamOwner(p *pool.Pool) error {
	_, err := auth.GetTeam(app.TeamOwner)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	poolTeams, err := p.GetTeams()
	if err != nil && err != pool.ErrPoolHasNoTeam {
		msg := fmt.Sprintf("failed to get pool %q teams", p.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	var poolTeam bool
	for _, team := range poolTeams {
		if team == app.TeamOwner {
			poolTeam = true
			break
		}
	}
	if !poolTeam {
		msg := fmt.Sprintf("App team owner %q has no access to pool %q", app.TeamOwner, p.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func (app *App) validateRouter(pool *pool.Pool) error {
	routers, err := pool.GetRouters()
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	for _, r := range routers {
		if r == app.Router {
			return nil
		}
	}
	msg := fmt.Sprintf("router %q is not available for pool %q. Available routers are: %q", app.Router, app.Pool, strings.Join(routers, ", "))
	return &tsuruErrors.ValidationError{Message: msg}
}

func (app *App) ValidateService(service string) error {
	pool, err := pool.GetPoolByName(app.Pool)
	if err != nil {
		return err
	}
	services, err := pool.GetServices()
	if err != nil {
		return err
	}
	for _, v := range services {
		if v == service {
			return nil
		}
	}
	msg := fmt.Sprintf("service %q is not available for pool %q. Available services are: %q", service, pool.Name, strings.Join(services, ", "))
	return &tsuruErrors.ValidationError{Message: msg}
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
	app.Log(fmt.Sprintf("running '%s'", cmd), "tsuru", "api")
	logWriter := LogWriter{App: app, Source: "app-run"}
	logWriter.Async()
	defer logWriter.Close()
	return app.sourced(cmd, io.MultiWriter(w, &logWriter), args)
}

func (app *App) sourced(cmd string, w io.Writer, args provision.RunArgs) error {
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := fmt.Sprintf("[ -d %s ] && cd %s", defaultAppDir, defaultAppDir)
	cmd = fmt.Sprintf("%s; %s; %s", source, cd, cmd)
	return app.run(cmd, w, args)
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
	if args.Isolated {
		return execProv.ExecuteCommandIsolated(w, w, app, cmd)
	}
	if args.Once {
		return execProv.ExecuteCommandOnce(w, w, app, cmd)
	}
	return execProv.ExecuteCommand(w, w, app, cmd)
}

// Restart runs the restart hook for the app, writing its output to w.
func (app *App) Restart(process string, w io.Writer) error {
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
	err = prov.Restart(app, process, w)
	if err != nil {
		log.Errorf("[restart] error on restart the app %s - %s", app.Name, err)
		return err
	}
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	return nil
}

func (app *App) Stop(w io.Writer, process string) error {
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
	err = prov.Stop(app, process)
	if err != nil {
		log.Errorf("[stop] error on stop the app %s - %s", app.Name, err)
		return err
	}
	return nil
}

func (app *App) Sleep(w io.Writer, process string, proxyURL *url.URL) error {
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
	r, err := app.GetRouter()
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		return err
	}
	oldRoutes, err := r.Routes(app.GetName())
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		return err
	}
	r.RemoveRoutes(app.GetName(), oldRoutes)
	err = r.AddRoutes(app.GetName(), []*url.URL{proxyURL})
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		return err
	}
	err = sleepProv.Sleep(app, process)
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		r.AddRoutes(app.GetName(), oldRoutes)
		r.RemoveRoutes(app.GetName(), []*url.URL{proxyURL})
		log.Errorf("[sleep] rolling back the sleep %s", app.Name)
		return err
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

// GetPool returns the pool of the app.
func (app *App) GetPool() string {
	return app.Pool
}

// GetTeamOwner returns the team owner of the app.
func (app *App) GetTeamOwner() string {
	return app.TeamOwner
}

// GetTeamsNames returns the names of teams app.
func (app *App) GetTeamsName() []string {
	return app.Teams
}

// GetMemory returns the memory limit (in bytes) for the app.
func (app *App) GetMemory() int64 {
	return app.Plan.Memory
}

// GetSwap returns the swap limit (in bytes) for the app.
func (app *App) GetSwap() int64 {
	return app.Plan.Swap
}

// GetCpuShare returns the cpu share for the app.
func (app *App) GetCpuShare() int {
	return app.Plan.CpuShare
}

// GetIp returns the ip of the app.
func (app *App) GetIp() string {
	return app.IP
}

func (app *App) GetQuota() quota.Quota {
	return app.Quota
}

func (app *App) SetQuotaInUse(inUse int) error {
	if inUse < 0 {
		return errors.New("invalid value, cannot be lesser than 0")
	}
	if !app.Quota.Unlimited() && inUse > app.Quota.Limit {
		return &quota.QuotaExceededError{
			Requested: uint(inUse),
			Available: uint(app.Quota.Limit),
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"quota.inuse": inUse}})
	if err == mgo.ErrNotFound {
		return ErrAppNotFound
	}
	return err
}

// GetCname returns the cnames of the app.
func (app *App) GetCname() []string {
	return app.CName
}

// GetLock returns the app lock information.
func (app *App) GetLock() provision.AppLock {
	return &app.Lock
}

// GetPlatform returns the platform of the app.
func (app *App) GetPlatform() string {
	return app.Platform
}

// GetDeploys returns the amount of deploys of an app.
func (app *App) GetDeploys() uint {
	return app.Deploys
}

// Envs returns a map representing the apps environment variables.
func (app *App) Envs() map[string]bind.EnvVar {
	mergedEnvs := make(map[string]bind.EnvVar, len(app.Env)+len(app.ServiceEnvs)+1)
	for _, e := range app.Env {
		mergedEnvs[e.Name] = e
	}
	for _, e := range app.ServiceEnvs {
		mergedEnvs[e.Name] = e.EnvVar
	}
	mergedEnvs[TsuruServicesEnvVar] = serviceEnvsFromEnvVars(app.ServiceEnvs)
	return mergedEnvs
}

// SetEnvs saves a list of environment variables in the app.
func (app *App) SetEnvs(setEnvs bind.SetEnvArgs) error {
	if len(setEnvs.Envs) == 0 {
		return nil
	}
	if setEnvs.Writer != nil {
		fmt.Fprintf(setEnvs.Writer, "---- Setting %d new environment variables ----\n", len(setEnvs.Envs))
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
		return app.restartIfUnits(setEnvs.Writer)
	}
	return nil
}

// UnsetEnvs removes environment variables from an app, serializing the
// remaining list of environment variables to all units of the app.
func (app *App) UnsetEnvs(unsetEnvs bind.UnsetEnvArgs) error {
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
		return app.restartIfUnits(unsetEnvs.Writer)
	}
	return nil
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
	return prov.Restart(app, "", w)
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
	err := action.NewPipeline(actions...).Execute(app, cnames)
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
	err := action.NewPipeline(actions...).Execute(app, cnames)
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

// Log adds a log message to the app. Specifying a good source is good so the
// user can filter where the message come from.
func (app *App) Log(message, source, unit string) error {
	messages := strings.Split(message, "\n")
	logs := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		if msg != "" {
			l := Applog{
				Date:    time.Now().In(time.UTC),
				Message: msg,
				Source:  source,
				AppName: app.Name,
				Unit:    unit,
			}
			logs = append(logs, l)
		}
	}
	if len(logs) > 0 {
		conn, err := db.LogConn()
		if err != nil {
			return err
		}
		defer conn.Close()
		return conn.Logs(app.Name).Insert(logs...)
	}
	return nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// fields in the log instance received as an example.
func (app *App) LastLogs(lines int, filterLog Applog) ([]Applog, error) {
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
	conn, err := db.LogConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	logs := []Applog{}
	q := bson.M{}
	if filterLog.Source != "" {
		q["source"] = filterLog.Source
	}
	if filterLog.Unit != "" {
		q["unit"] = filterLog.Unit
	}
	err = conn.Logs(app.Name).Find(q).Sort("-$natural").Limit(lines).All(&logs)
	if err != nil {
		return nil, err
	}
	l := len(logs)
	for i := 0; i < l/2; i++ {
		logs[i], logs[l-1-i] = logs[l-1-i], logs[i]
	}
	return logs, nil
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
		query["framework"] = f.Platform
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

// List returns the list of apps filtered through the filter parameter.
func List(filter *Filter) ([]App, error) {
	apps := []App{}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	query := filter.Query()
	if err = conn.Apps().Find(query).All(&apps); err != nil {
		return apps, err
	}
	if filter != nil && len(filter.Statuses) > 0 {
		appsProvisionerMap := make(map[string][]provision.App)
		for i := range apps {
			a := &apps[i]
			prov, err := a.getProvisioner()
			if err != nil {
				return nil, err
			}
			appsProvisionerMap[prov.GetName()] = append(appsProvisionerMap[prov.GetName()], a)
		}
		var provisionApps []provision.App
		for provName, apps := range appsProvisionerMap {
			prov, err := provision.Get(provName)
			if err != nil {
				return nil, err
			}
			if filterProv, ok := prov.(provision.AppFilterProvisioner); ok {
				apps, err = filterProv.FilterAppsByUnitStatus(apps, filter.Statuses)
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
	return apps, nil
}

// Swap calls the Router.Swap and updates the app.CName in the database.
func Swap(app1, app2 *App, cnameOnly bool) error {
	r1, err := app1.GetRouter()
	if err != nil {
		return err
	}
	r2, err := app2.GetRouter()
	if err != nil {
		return err
	}
	defer rebuild.RoutesRebuildOrEnqueue(app1.Name)
	defer rebuild.RoutesRebuildOrEnqueue(app2.Name)
	err = r1.Swap(app1.Name, app2.Name, cnameOnly)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app1.CName, app2.CName = app2.CName, app1.CName
	updateCName := func(app *App, r router.Router) error {
		app.IP, err = r.Addr(app.Name)
		if err != nil {
			return err
		}
		return conn.Apps().Update(
			bson.M{"name": app.Name},
			bson.M{"$set": bson.M{"cname": app.CName, "ip": app.IP}},
		)
	}
	err = updateCName(app1, r1)
	if err != nil {
		return err
	}
	return updateCName(app2, r2)
}

// Start starts the app calling the provisioner.Start method and
// changing the units state to StatusStarted.
func (app *App) Start(w io.Writer, process string) error {
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
	err = prov.Start(app, process)
	if err != nil {
		log.Errorf("[start] error on start the app %s - %s", app.Name, err)
		return err
	}
	rebuild.RoutesRebuildOrEnqueue(app.Name)
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

func (app *App) RegisterUnit(unitId string, customData map[string]interface{}) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	err = prov.RegisterUnit(app, unitId, customData)
	if err != nil {
		return err
	}
	units, err := prov.Units(app)
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

func (app *App) GetRouterName() (string, error) {
	return app.Router, nil
}

func (app *App) GetRouter() (router.Router, error) {
	return router.Get(app.Router)
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

func (app *App) Shell(opts provision.ShellOptions) error {
	opts.App = app
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	if shellProv, ok := prov.(provision.ShellProvisioner); ok {
		return shellProv.Shell(opts)
	} else {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "running shell"}
	}
}

func (app *App) SetCertificate(name, certificate, key string) error {
	hasCname := false
	for _, c := range app.CName {
		if c == name {
			hasCname = true
		}
	}
	if !hasCname && name != app.IP {
		return errors.New("invalid name")
	}
	r, err := app.GetRouter()
	if err != nil {
		return err
	}
	tlsRouter, ok := r.(router.TLSRouter)
	if !ok {
		return errors.New("router does not support tls")
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
	return tlsRouter.AddCertificate(name, certificate, key)
}

func (app *App) RemoveCertificate(name string) error {
	hasCname := false
	for _, c := range app.CName {
		if c == name {
			hasCname = true
		}
	}
	if !hasCname && name != app.IP {
		return errors.New("invalid name")
	}
	r, err := app.GetRouter()
	if err != nil {
		return err
	}
	tlsRouter, ok := r.(router.TLSRouter)
	if !ok {
		return errors.New("router does not support tls")
	}
	return tlsRouter.RemoveCertificate(name)
}

func (app *App) GetCertificates() (map[string]string, error) {
	r, err := app.GetRouter()
	if err != nil {
		return nil, err
	}
	tlsRouter, ok := r.(router.TLSRouter)
	if !ok {
		return nil, errors.New("router does not support tls")
	}
	names := append(app.CName, app.IP)
	certificates := make(map[string]string)
	for _, n := range names {
		cert, err := tlsRouter.GetCertificate(n)
		if err != nil && err != router.ErrCertificateNotFound {
			return nil, err
		}
		certificates[n] = cert
	}
	return certificates, nil
}

type ProcfileError struct {
	yamlErr error
}

func (e *ProcfileError) Error() string {
	return fmt.Sprintf("error parsing Procfile: %s", e.yamlErr)
}

func (app *App) UpdateAddr() error {
	r, err := app.GetRouter()
	if err != nil {
		return err
	}
	newAddr, err := r.Addr(app.Name)
	if err != nil {
		return err
	}
	if newAddr == app.IP {
		return nil
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"ip": newAddr}})
	if err != nil {
		return err
	}
	app.IP = newAddr
	return nil
}

func (app *App) RoutableAddresses() ([]url.URL, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	return prov.RoutableAddresses(app)
}

func (app *App) InternalLock(reason string) (bool, error) {
	return AcquireApplicationLock(app.Name, InternalAppName, reason)
}

func (app *App) Unlock() {
	ReleaseApplicationLock(app.Name)
}

func (app *App) withLogWriter(w io.Writer) io.Writer {
	logWriter := &LogWriter{App: app}
	if w != nil {
		w = io.MultiWriter(w, logWriter)
	} else {
		w = logWriter
	}
	return w
}

func RenameTeam(oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	filter := &Filter{}
	filter.ExtraIn("teams", oldName)
	filter.ExtraIn("teamowner", oldName)
	apps, err := List(filter)
	if err != nil {
		return err
	}
	for _, a := range apps {
		var locked bool
		locked, err = AcquireApplicationLock(a.Name, InternalAppName, "team rename")
		if err != nil {
			return err
		}
		if !locked {
			return errors.Errorf("unable to acquire lock for app %q", a.Name)
		}
		defer ReleaseApplicationLock(a.Name)
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
