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
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var AuthScheme auth.Scheme

var (
	nameRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

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
	Platform       string `bson:"framework"`
	Name           string
	Ip             string
	CName          []string
	Teams          []string
	TeamOwner      string
	Owner          string
	Plan           Plan
	UpdatePlatform bool
	Lock           AppLock
	Pool           string
	Description    string
	Router         string
	RouterOpts     map[string]string
	Deploys        uint
	Tags           []string

	quota.Quota
	provisioner provision.Provisioner
}

func (app *App) getProvisioner() (provision.Provisioner, error) {
	if app.provisioner == nil {
		if app.Pool == "" {
			return provision.GetDefault()
		}
		pool, err := provision.GetPoolByName(app.Pool)
		if err != nil {
			return nil, err
		}
		app.provisioner, err = pool.GetProvisioner()
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
		return nil, err
	}
	return prov.Units(app)
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
	if err != nil {
		return nil, err
	}
	result["units"] = units
	result["repository"] = repo.ReadWriteURL
	result["ip"] = app.Ip
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
	result["lock"] = app.Lock
	return json.Marshal(&result)
}

// Applog represents a log entry.
type Applog struct {
	Date    time.Time
	Message string
	Source  string
	AppName string
	Unit    string
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
	var plan *Plan
	var err error
	if app.Plan.Name == "" {
		plan, err = DefaultPlan()
	} else {
		plan, err = findPlanByName(app.Plan.Name)
	}
	if err != nil {
		return err
	}
	if app.Router == "" {
		app.Router, err = router.Default()
	} else {
		_, err = router.Get(app.Router)
	}
	if err != nil {
		return err
	}
	app.Plan = *plan
	err = app.SetPool()
	if err != nil {
		return err
	}
	app.Teams = []string{app.TeamOwner}
	app.Owner = user.Email
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
	tags := updateData.Tags
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
	err = app.validate()
	if err != nil {
		return err
	}
	if app.Router != oldRouter || app.Plan != oldPlan {
		actions := []*action.Action{
			&moveRouterUnits,
			&saveApp,
			&restartApp,
			&removeOldBackend,
		}
		err = action.NewPipeline(actions...).Execute(app, &oldPlan, oldRouter, w)
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

// unbind takes all service instances that are bound to the app, and unbind
// them. This method is used by Destroy (before destroying the app, it unbinds
// all service instances). Refer to Destroy docs for more details.
func (app *App) unbind() error {
	instances, err := app.serviceInstances()
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
	instances, err := app.serviceInstances()
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
	instances, err := app.serviceInstances()
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

func (app *App) serviceInstances() ([]service.ServiceInstance, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instances []service.ServiceInstance
	q := bson.M{"apps": bson.M{"$in": []string{app.Name}}}
	err = conn.ServiceInstances().Find(q).All(&instances)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (app *App) AddUnits(n uint, process string, writer io.Writer) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
	}
	err := action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
	).Execute(app, n, writer, process)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	return err
}

// RemoveUnits removes n units from the app. It's a process composed of
// multiple steps:
//
//     1. Remove units from the provisioner
//     2. Update quota
func (app *App) RemoveUnits(n uint, process string, writer io.Writer) error {
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	err = prov.RemoveUnits(app, n, process, writer)
	rebuild.RoutesRebuildOrEnqueue(app.Name)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	units, err := app.Units()
	if err != nil {
		return err
	}
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{
			"$set": bson.M{
				"quota.inuse": len(units),
			},
		},
	)
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

func (app *App) findTeam(team *auth.Team) (int, bool) {
	for i, teamName := range app.Teams {
		if teamName == team.Name {
			return i, true
		}
	}
	return -1, false
}

// Grant allows a team to have access to an app. It returns an error if the
// team already have access to the app.
func (app *App) Grant(team *auth.Team) error {
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
func (app *App) Revoke(team *auth.Team) error {
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
func (app *App) GetTeams() []auth.Team {
	var teams []auth.Team
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
		return nil
	}
	defer conn.Close()
	conn.Teams().Find(bson.M{"_id": bson.M{"$in": app.Teams}}).All(&teams)
	return teams
}

func (app *App) SetPool() error {
	poolName, err := app.getPoolForApp(app.Pool)
	if err != nil {
		return err
	}
	if poolName == "" {
		var pool *provision.Pool
		pool, err = provision.GetDefaultPool()
		if err != nil {
			return err
		}
		poolName = pool.Name
	}
	app.Pool = poolName
	pool, err := provision.GetPoolByName(poolName)
	if err != nil {
		return err
	}
	return app.validateTeamOwner(pool)
}

func (app *App) getPoolForApp(poolName string) (string, error) {
	if poolName == "" {
		pools, err := provision.ListPoolsForTeam(app.TeamOwner)
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
	pool, err := provision.GetPoolByName(poolName)
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
	var (
		env bind.EnvVar
		err error
		ok  bool
	)
	if env, ok = app.Env[name]; !ok {
		err = errors.New("Environment variable not declared for this app.")
	}
	return env, err
}

// validate checks app name format
func (app *App) validate() error {
	if app.Name == InternalAppName || !nameRegexp.MatchString(app.Name) {
		msg := "Invalid app name, your app should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return app.validatePool()
}

func (app *App) validatePool() error {
	pool, err := provision.GetPoolByName(app.Pool)
	if err != nil {
		return err
	}
	err = app.validateTeamOwner(pool)
	if err != nil {
		return err
	}
	return app.validateRouter(pool)
}

func (app *App) validateTeamOwner(pool *provision.Pool) error {
	_, err := auth.GetTeam(app.TeamOwner)
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	poolTeams, err := pool.GetTeams()
	if err != nil && err != provision.ErrPoolHasNoTeam {
		msg := fmt.Sprintf("failed to get pool %q teams", pool.Name)
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
		msg := fmt.Sprintf("App team owner %q has no access to pool %q", app.TeamOwner, pool.Name)
		return &tsuruErrors.ValidationError{Message: msg}
	}
	return nil
}

func (app *App) validateRouter(pool *provision.Pool) error {
	routers, err := pool.GetRouters()
	if err != nil {
		return &tsuruErrors.ValidationError{Message: err.Error()}
	}
	for _, r := range routers {
		if r == app.Router {
			return nil
		}
	}
	msg := fmt.Sprintf("router %q is not available for pool %q", app.Router, app.Pool)
	return &tsuruErrors.ValidationError{Message: msg}
}

// InstanceEnv returns a map of environment variables that belongs to the given
// service instance (identified by the name only).
func (app *App) InstanceEnv(name string) map[string]bind.EnvVar {
	envs := make(map[string]bind.EnvVar)
	for k, env := range app.Env {
		if env.InstanceName == name {
			envs[k] = env
		}
	}
	return envs
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (app *App) Run(cmd string, w io.Writer, args provision.RunArgs) error {
	if !app.available() {
		return errors.New("App must be available to run commands")
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
	msg := fmt.Sprintf("---- Restarting process %q ----\n", process)
	if process == "" {
		msg = fmt.Sprintf("---- Restarting the app %q ----\n", app.Name)
	}
	err := log.Write(w, []byte(msg))
	if err != nil {
		log.Errorf("[restart] error on write app log for the app %s - %s", app.Name, err)
		return err
	}
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
	msg := fmt.Sprintf("\n ---> Stopping the process %q\n", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Stopping the app %q\n", app.Name)
	}
	log.Write(w, []byte(msg))
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
	msg := fmt.Sprintf("\n ---> Putting the process %q to sleep\n", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Putting the app %q to sleep\n", app.Name)
	}
	log.Write(w, []byte(msg))
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
	for _, route := range oldRoutes {
		r.RemoveRoute(app.GetName(), route)
	}
	err = r.AddRoute(app.GetName(), proxyURL)
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		return err
	}
	err = sleepProv.Sleep(app, process)
	if err != nil {
		log.Errorf("[sleep] error on sleep the app %s - %s", app.Name, err)
		for _, route := range oldRoutes {
			r.AddRoute(app.GetName(), route)
		}
		r.RemoveRoute(app.GetName(), proxyURL)
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
	return app.Ip
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
	return app.Env
}

// SetEnvs saves a list of environment variables in the app. The publicOnly
// parameter indicates whether only public variables can be overridden (if set
// to false, SetEnvs may override a private variable).
func (app *App) SetEnvs(setEnvs bind.SetEnvApp, w io.Writer) error {
	units, err := app.GetUnits()
	if err != nil {
		return err
	}
	if len(units) > 0 {
		return app.setEnvsToApp(setEnvs, w)
	}
	setEnvs.ShouldRestart = false
	return app.setEnvsToApp(setEnvs, w)
}

// setEnvsToApp adds environment variables to an app, serializing the resulting
// list of environment variables in all units of apps. This method can
// serialize them directly or using a queue.
//
// Besides the slice of environment variables, this method also takes two other
// parameters: publicOnly indicates whether only public variables can be
// overridden (if set to false, setEnvsToApp may override a private variable).
//
// shouldRestart defines if the server should be restarted after saving vars.
func (app *App) setEnvsToApp(setEnvs bind.SetEnvApp, w io.Writer) error {
	if len(setEnvs.Envs) == 0 {
		return nil
	}
	if w != nil {
		fmt.Fprintf(w, "---- Setting %d new environment variables ----\n", len(setEnvs.Envs))
	}
	for _, env := range setEnvs.Envs {
		set := true
		if setEnvs.PublicOnly {
			e, err := app.getEnv(env.Name)
			if err == nil && !e.Public && e.InstanceName != "" {
				set = false
			}
		}
		if set {
			app.setEnv(env)
		}
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
	if !setEnvs.ShouldRestart {
		return nil
	}
	prov, err := app.getProvisioner()
	if err != nil {
		return err
	}
	return prov.Restart(app, "", w)
}

// UnsetEnvs removes environment variables from an app, serializing the
// remaining list of environment variables to all units of the app.
//
// Besides the slice with the name of the variables, this method also takes the
// parameter publicOnly, which indicates whether only public variables can be
// overridden (if set to false, setEnvsToApp may override a private variable).
func (app *App) UnsetEnvs(unsetEnvs bind.UnsetEnvApp, w io.Writer) error {
	units, err := app.GetUnits()
	if err != nil {
		return err
	}
	if len(units) > 0 {
		return app.unsetEnvsToApp(unsetEnvs, w)
	}
	unsetEnvs.ShouldRestart = false
	return app.unsetEnvsToApp(unsetEnvs, w)
}

func (app *App) unsetEnvsToApp(unsetEnvs bind.UnsetEnvApp, w io.Writer) error {
	if len(unsetEnvs.VariableNames) == 0 {
		return nil
	}
	if w != nil {
		fmt.Fprintf(w, "---- Unsetting %d environment variables ----\n", len(unsetEnvs.VariableNames))
	}
	for _, name := range unsetEnvs.VariableNames {
		var unset bool
		e, err := app.getEnv(name)
		if !unsetEnvs.PublicOnly || (err == nil && e.Public) {
			unset = true
		}
		if unset {
			delete(app.Env, name)
		}
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
	if !unsetEnvs.ShouldRestart {
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

func (app *App) parsedTsuruServices() map[string][]bind.ServiceInstance {
	var tsuruServices map[string][]bind.ServiceInstance
	if servicesEnv, ok := app.Env[TsuruServicesEnvVar]; ok {
		json.Unmarshal([]byte(servicesEnv.Value), &tsuruServices)
	} else {
		tsuruServices = make(map[string][]bind.ServiceInstance)
	}
	return tsuruServices
}

//func (app *App) AddInstance(serviceName string, instance bind.ServiceInstance, shouldRestart bool, writer io.Writer) error {
func (app *App) AddInstance(instanceApp bind.InstanceApp, writer io.Writer) error {
	tsuruServices := app.parsedTsuruServices()
	serviceInstances := appendOrUpdateServiceInstance(tsuruServices[instanceApp.ServiceName], instanceApp.Instance)
	tsuruServices[instanceApp.ServiceName] = serviceInstances
	servicesJson, err := json.Marshal(tsuruServices)
	if err != nil {
		return err
	}
	if len(instanceApp.Instance.Envs) == 0 {
		return nil
	}
	envVars := make([]bind.EnvVar, 0, len(instanceApp.Instance.Envs)+1)
	for k, v := range instanceApp.Instance.Envs {
		envVars = append(envVars, bind.EnvVar{
			Name:         k,
			Value:        v,
			Public:       false,
			InstanceName: instanceApp.Instance.Name,
		})
	}
	envVars = append(envVars, bind.EnvVar{
		Name:   TsuruServicesEnvVar,
		Value:  string(servicesJson),
		Public: false,
	})
	return app.SetEnvs(
		bind.SetEnvApp{
			Envs:          envVars,
			PublicOnly:    false,
			ShouldRestart: instanceApp.ShouldRestart,
		}, writer)
}

func appendOrUpdateServiceInstance(services []bind.ServiceInstance, service bind.ServiceInstance) []bind.ServiceInstance {
	serviceInstanceFound := false
	for i, serviceInstance := range services {
		if serviceInstance.Name == service.Name {
			serviceInstanceFound = true
			services[i] = service
			break
		}
	}
	if !serviceInstanceFound {
		services = append(services, service)
	}
	return services
}

func findServiceEnv(tsuruServices map[string][]bind.ServiceInstance, name string) (string, string) {
	for _, serviceInstances := range tsuruServices {
		for _, instance := range serviceInstances {
			if instance.Envs[name] != "" {
				return instance.Name, instance.Envs[name]
			}
		}
	}
	return "", ""
}

//func (app *App) RemoveInstance(serviceName string, instance bind.ServiceInstance, shouldRestart bool, writer io.Writer) error {
func (app *App) RemoveInstance(instanceApp bind.InstanceApp, writer io.Writer) error {
	tsuruServices := app.parsedTsuruServices()
	toUnsetEnvs := make([]string, 0, len(instanceApp.Instance.Envs))
	for varName := range instanceApp.Instance.Envs {
		toUnsetEnvs = append(toUnsetEnvs, varName)
	}
	index := -1
	serviceInstances := tsuruServices[instanceApp.ServiceName]
	for i, si := range serviceInstances {
		if si.Name == instanceApp.Instance.Name {
			index = i
			break
		}
	}
	var servicesJson []byte
	var err error
	if index >= 0 {
		for i := index; i < len(serviceInstances)-1; i++ {
			serviceInstances[i] = serviceInstances[i+1]
		}
		tsuruServices[instanceApp.ServiceName] = serviceInstances[:len(serviceInstances)-1]
		servicesJson, err = json.Marshal(tsuruServices)
		if err != nil {
			return err
		}
	}
	var envsToSet []bind.EnvVar
	for _, varName := range toUnsetEnvs {
		instanceName, envValue := findServiceEnv(tsuruServices, varName)
		if envValue == "" || instanceName == "" {
			break
		}
		envsToSet = append(envsToSet, bind.EnvVar{
			Name:         varName,
			Value:        envValue,
			Public:       false,
			InstanceName: instanceName,
		})
	}
	if servicesJson != nil {
		envsToSet = append(envsToSet, bind.EnvVar{
			Name:   TsuruServicesEnvVar,
			Value:  string(servicesJson),
			Public: false,
		})
	}
	if len(toUnsetEnvs) > 0 {
		units, err := app.GetUnits()
		if err != nil {
			return err
		}
		restart := instanceApp.ShouldRestart
		if instanceApp.ShouldRestart {
			restart = len(envsToSet) == 0 && len(units) > 0
		}
		err = app.unsetEnvsToApp(
			bind.UnsetEnvApp{
				VariableNames: toUnsetEnvs,
				PublicOnly:    false,
				ShouldRestart: restart,
			}, writer)
		if err != nil {
			return err
		}
	}
	return app.SetEnvs(
		bind.SetEnvApp{
			Envs:          envsToSet,
			PublicOnly:    false,
			ShouldRestart: instanceApp.ShouldRestart,
		}, writer)
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
		notify(app.Name, logs)
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
		app.Ip, err = r.Addr(app.Name)
		if err != nil {
			return err
		}
		return conn.Apps().Update(
			bson.M{"name": app.Name},
			bson.M{"$set": bson.M{"cname": app.CName, "ip": app.Ip}},
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
	msg := fmt.Sprintf("\n ---> Starting the process %q\n", process)
	if process == "" {
		msg = fmt.Sprintf("\n ---> Starting the app %q\n", app.Name)
	}
	log.Write(w, []byte(msg))
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
	return prov.RegisterUnit(app, unitId, customData)
}

func (app *App) GetRouterName() (string, error) {
	return app.Router, nil
}

func (app *App) GetRouter() (router.Router, error) {
	return router.Get(app.Router)
}

func (app *App) MetricEnvs() (map[string]string, error) {
	prov, err := app.getProvisioner()
	if err != nil {
		return nil, err
	}
	if metricProv, ok := prov.(provision.MetricsProvisioner); ok {
		return metricProv.MetricEnvs(app), nil
	} else {
		return nil, provision.ProvisionerNotSupported{Prov: prov, Action: "metrics"}
	}
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
	if !hasCname && name != app.Ip {
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
	if !hasCname && name != app.Ip {
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
	names := append(app.CName, app.Ip)
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
	if newAddr == app.Ip {
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
	app.Ip = newAddr
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
