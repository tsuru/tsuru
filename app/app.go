// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	stderr "errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var Provisioner provision.Provisioner

var (
	nameRegexp  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	cnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][\w-.]+$`)
)

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App struct {
	Env       map[string]bind.EnvVar
	Platform  string `bson:"framework"`
	Name      string
	Ip        string
	CName     string
	Units     []Unit
	Teams     []string
	TeamOwner string
	Owner     string
	State     string
	Deploys   uint
	Memory    int `json:",string"`
	Swap      int `json:",string"`

	quota.Quota
	hr hookRunner
}

// MarshalJSON marshals the app in json format. It returns a JSON object with
// the following keys: name, framework, teams, units, repository and ip.
func (app *App) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})
	result["name"] = app.Name
	result["platform"] = app.Platform
	result["teams"] = app.Teams
	result["units"] = app.Units
	result["repository"] = repository.ReadWriteURL(app.Name)
	result["ip"] = app.Ip
	result["cname"] = app.CName
	result["ready"] = app.State == "ready"
	result["owner"] = app.Owner
	result["deploys"] = app.Deploys
	result["memory"] = strconv.Itoa(app.Memory)
	result["swap"] = "0"
	if app.Swap > 0 {
		result["swap"] = strconv.Itoa(app.Swap - app.Memory)
	}
	return json.Marshal(&result)
}

// Applog represents a log entry.
type Applog struct {
	Date    time.Time
	Message string
	Source  string
	AppName string
}

// GetAppByName queries the database to find an app identified by the given
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
// Creating a new app is a process composed of four steps:
//
//       1. Save the app in the database
//       2. Create IAM credentials for the app
//       3. Create the git repository using gandalf
//       4. Provision units within the provisioner
func CreateApp(app *App, user *auth.User) error {
	teams, err := user.Teams()
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		return NoTeamsError{}
	}
	if _, err := getPlatform(app.Platform); err != nil {
		return err
	}
	// app.Memory is empty, no custom memory passed from CLI
	if app.Memory < 1 {
		// get default memory limit from tsuru config
		configMemory, err := config.GetInt("docker:memory")
		if err != nil {
			// no default memory set in config (or error when reading), set it as unlimited (0)
			app.Memory = 0
		} else {
			// default memory set in config, use that.
			app.Memory = configMemory
		}
	}
	// app.Swap is empty, no custom swap passed from CLI
	if app.Swap < 1 {
		// get default swap limit from tsuru config
		configSwap, err := config.GetInt("docker:swap")
		if err != nil {
			// no default swap set in config (or error when reading), set it as unlimited (0)
			app.Swap = 0
		} else {
			// default swap set in config, use that.
			app.Swap = configSwap
		}
	}
	// Swap size must always be the sum of memory plus swap
	if app.Swap > 0 {
		app.Swap = app.Memory + app.Swap
	}
	if err := app.setTeamOwner(teams); err != nil {
		return err
	}
	app.SetTeams(teams)
	app.Owner = user.Email
	if !app.isValid() {
		msg := "Invalid app name, your app should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &errors.ValidationError{Message: msg}
	}
	if app.equalAppNameAndPlatformName() {
		msg := "Invalid app name: platform name and app name " +
			"can not be the same"
		return &errors.ValidationError{Message: msg}
	}
	if app.equalToSomePlatformName() {
		msg := "Invalid app name: platform name already exists " +
			"with the same name"
		return &errors.ValidationError{Message: msg}
	}
	actions := []*action.Action{
		&reserveUserApp,
		&insertApp,
		&exportEnvironmentsAction,
		&createRepository,
		&provisionApp,
	}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, user)
	if err != nil {
		return &AppCreationError{app: app.Name, Err: err}
	}
	app.Ip, err = Provisioner.Addr(app)
	if err != nil {
		errMsg := "Failed to obtain app %s address: %s"
		log.Errorf(errMsg, app.GetName(), err)
		return fmt.Errorf(errMsg, app.GetName(), err)
	}
	return nil
}

// unbind takes all service instances that are bound to the app, and unbind
// them. This method is used by Destroy (before destroying the app, it unbinds
// all service instances). Refer to Destroy docs for more details.
func (app *App) unbind() error {
	var instances []service.ServiceInstance
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	q := bson.M{"apps": bson.M{"$in": []string{app.Name}}}
	err = conn.ServiceInstances().Find(q).All(&instances)
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
		err = instance.UnbindApp(app)
		if err != nil {
			addMsg(instance.Name, err)
		}
	}
	if msg != "" {
		return stderr.New(msg)
	}
	return nil
}

// Delete deletes an app.
//
// Delete an app is a process composed of four steps:
//
//       1. Destroy the app unit
//       2. Unbind all service instances from the app
//       4. Remove the app from the database
func Delete(app *App) error {
	gURL := repository.ServerURL()
	(&gandalf.Client{Endpoint: gURL}).RemoveRepository(app.Name)
	if len(app.Units) > 0 {
		Provisioner.Destroy(app)
		app.unbind()
	}
	token := app.Env["TSURU_APP_TOKEN"].Value
	auth.DeleteToken(token)
	if owner, err := auth.GetUserByEmail(app.Owner); err == nil {
		auth.ReleaseApp(owner)
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Remove(bson.M{"name": app.Name})
}

// Add provisioned units to database and enqueue messages to
// bind services and regenerate apprc. It's called as one of
// the steps started by AddUnits(). It doesn't call the
// provisioner.
func (app *App) AddUnitsToDB(units []provision.Unit) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	messages := make([]queue.Message, len(units)*2)
	mCount := 0
	for _, unit := range units {
		unit := Unit{
			Name:       unit.Name,
			Type:       unit.Type,
			Ip:         unit.Ip,
			Machine:    unit.Machine,
			State:      provision.StatusBuilding.String(),
			InstanceId: unit.InstanceId,
		}
		app.AddUnit(&unit)
		messages[mCount] = queue.Message{Action: regenerateApprc, Args: []string{app.Name, unit.Name}}
		messages[mCount+1] = queue.Message{Action: BindService, Args: []string{app.Name, unit.Name}}
		mCount += 2
	}
	err = conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"units": app.Units}},
	)
	if err != nil {
		return err
	}
	go Enqueue(messages...)
	return nil
}

// AddUnit adds a new unit to the app (or update an existing unit). It just updates
// the internal list of units, it does not talk to the provisioner. For
// provisioning a new unit for the app, one should use AddUnits method, which
// receives the number of units that you want to provision.
func (app *App) AddUnit(u *Unit) {
	for i, unt := range app.Units {
		if unt.Name == u.Name {
			app.Units[i] = *u
			return
		} else if unt.Name == "" {
			app.Units[i] = *u
			return
		}
	}
	app.Units = append(app.Units, *u)
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (app *App) AddUnits(n uint) error {
	if n == 0 {
		return stderr.New("Cannot add zero units.")
	}
	err := action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
		&saveNewUnitsInDatabase,
	).Execute(app, n)
	return err
}

// RemoveUnit removes a unit by its InstanceId or Name.
//
// Will search first by InstanceId, if no instance is found, then tries to
// search using the unit name, then calls the removal function from provisioner
//
// Returns an error in case of failure.
func (app *App) RemoveUnit(id string) error {
	unit, i, err := app.findUnitByID(id)
	if err != nil {
		return err
	}
	if err = Provisioner.RemoveUnit(app, unit.GetName()); err != nil {
		log.Error(err.Error())
	}
	return app.removeUnitByIdx(i, unit)
}

// RemoveUnitFromDB removes a unit only from database. It doesn't call the
// provisioner.
// An error might be returned in case of failure.
func (app *App) RemoveUnitFromDB(id string) error {
	unit, i, err := app.findUnitByID(id)
	if err != nil {
		return err
	}
	return app.removeUnitByIdx(i, unit)
}

// findUnitByID searchs first by InstanceId, if no instance is found, then tries to
// search using the unit name.
// It returns the Unit instance and its index inside the the app.Units list.
// An error might be returned in case of failure.
func (app *App) findUnitByID(id string) (*Unit, int, error) {
	var (
		unit Unit
		i    int
		u    Unit
	)
	for i, u = range app.Units {
		if u.InstanceId == id || u.Name == id {
			unit = u
			break
		}
	}
	if unit.GetName() == "" {
		return nil, 0, stderr.New(fmt.Sprintf("Unit not found: %s.", id))
	}
	return &unit, i, nil
}

func (app *App) removeUnitByIdx(i int, unit provision.AppUnit) error {
	app.removeUnits([]int{i})
	app.unbindUnit(unit)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"units": app.Units}},
	)
}

// removeUnits removes units identified by the given indices. The slice of
// indices must be sorted in ascending order. If the slice is unsorted, the
// behavior of the method is unknown.
//
// For example, if the app have the following units:
//
//     {"unit1", "unit2", "unit3", "unit4", "unit5"}
//
// Calling this method with with the slice []int{2, 4} would remove "unit3" and
// "unit5" from the list.
func (app *App) removeUnits(indices []int) {
	prefix := true
	for i := range indices {
		if i != indices[i] {
			prefix = false
			break
		}
	}
	if prefix {
		app.Units = app.Units[len(indices):]
	} else {
		for i, index := range indices {
			index -= i
			if index+1 < len(app.Units) {
				copy(app.Units[index:], app.Units[index+1:])
			}
			app.Units = app.Units[:len(app.Units)-1]
		}
	}
}

// RemoveUnits removes n units from the app. It's a process composed of x
// steps:
//
//     1. Remove units from the provisioner
//     2. Unbind units from service instances bound to the app
//     3. Remove units from the app list
//     4. Update the app in the database
func (app *App) RemoveUnits(n uint) error {
	if n == 0 {
		return stderr.New("Cannot remove zero units.")
	} else if l := uint(len(app.Units)); l == n {
		return stderr.New("Cannot remove all units from an app.")
	} else if n > l {
		return fmt.Errorf("Cannot remove %d units from this app, it has only %d units.", n, l)
	}
	var (
		removed []int
		err     error
	)
	units := UnitSlice(app.Units)
	sort.Sort(units)
	for i := 0; i < int(n); i++ {
		name := units[i].GetName()
		go Provisioner.RemoveUnit(app, name)
		removed = append(removed, i)
		app.unbindUnit(&units[i])
	}
	if len(removed) == 0 {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app.removeUnits(removed)
	dbErr := conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{
			"$set": bson.M{
				"units":       app.Units,
				"quota.inuse": len(app.Units),
			},
		},
	)
	if err == nil {
		return dbErr
	}
	return err
}

// unbindUnit unbinds a unit from all service instances that are bound to the
// app. It is used by RemoveUnit and RemoveUnits methods.
func (app *App) unbindUnit(unit provision.AppUnit) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var instances []service.ServiceInstance
	q := bson.M{"apps": bson.M{"$in": []string{app.Name}}}
	err = conn.ServiceInstances().Find(q).All(&instances)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		go func(instance service.ServiceInstance, unit provision.AppUnit) {
			err = instance.UnbindUnit(unit)
			if err != nil {
				log.Errorf("Error unbinding the unit %s with the service instance %s.", unit.GetIp(), instance.Name)
			}
		}(instance, unit)
	}
	return nil
}

// Available returns true if at least one of N units is started or unreachable.
func (app *App) Available() bool {
	for _, unit := range app.ProvisionedUnits() {
		if unit.Available() {
			return true
		}
	}
	return false
}

// Find returns the position of the given team in the internal list of teams
// that have access to the app. It returns an integer and a boolean, the
// boolean indicates whether the teams was found, and the integer indicates
// where it was found.
//
// Its's implemented after sort.Search. That's why it works this way.
func (app *App) find(team *auth.Team) (int, bool) {
	pos := sort.Search(len(app.Teams), func(i int) bool {
		return app.Teams[i] >= team.Name
	})
	return pos, pos < len(app.Teams) && app.Teams[pos] == team.Name
}

// Grant allows a team to have access to an app. It returns an error if the
// team already have access to the app.
func (app *App) Grant(team *auth.Team) error {
	pos, found := app.find(team)
	if found {
		return stderr.New("This team already has access to this app")
	}
	app.Teams = append(app.Teams, "")
	tmp := app.Teams[pos]
	for i := pos; i < len(app.Teams)-1; i++ {
		app.Teams[i+1], tmp = tmp, app.Teams[i]
	}
	app.Teams[pos] = team.Name
	return nil
}

// Revoke removes the access from a team. It returns an error if the team do
// not have access to the app.
func (app *App) Revoke(team *auth.Team) error {
	index, found := app.find(team)
	if !found {
		return stderr.New("This team does not have access to this app")
	}
	copy(app.Teams[index:], app.Teams[index+1:])
	app.Teams = app.Teams[:len(app.Teams)-1]
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

// SetTeams sets the values of the internal te
//
// TODO(fss): this method should not be exported.
func (app *App) SetTeams(teams []auth.Team) {
	app.Teams = make([]string, len(teams))
	for i, team := range teams {
		app.Teams[i] = team.Name
	}
	sort.Strings(app.Teams)
}

// setTeamOwner sets the TeamOwner value.
func (app *App) setTeamOwner(teams []auth.Team) error {
	if app.TeamOwner == "" {
		if len(teams) > 1 {
			return ManyTeamsError{}
		}
		app.TeamOwner = teams[0].Name
	} else {
		for _, t := range teams {
			if t.Name == app.TeamOwner {
				return nil
			}
		}
		return stderr.New("team not found.")
	}
	return nil
}

// setEnv sets the given environment variable in the app.
func (app *App) setEnv(env bind.EnvVar) {
	if app.Env == nil {
		app.Env = make(map[string]bind.EnvVar)
	}
	app.Env[env.Name] = env
	if env.Public {
		app.Log(fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru")
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
		err = stderr.New("Environment variable not declared for this app.")
	}
	return env, err
}

// isValid indicates whether the name of the app is valid.
func (app *App) isValid() bool {
	return nameRegexp.MatchString(app.Name)
}

// equalAppNameAndPlatformName check if the app.Name and app.Platform have
// same name
func (app *App) equalAppNameAndPlatformName() bool {
	return app.Name == app.GetPlatform()
}

// equalToSomePlatformName indicates if app.Name and some Platform are equals
func (app *App) equalToSomePlatformName() bool {
	platforms, err := Platforms()
	if err != nil {
		return false
	}
	for _, platform := range platforms {
		if app.Name == platform.Name {
			return true
		}
	}
	return false
}

// InstanceEnv returns a map of environment variables that belongs to the given
// service instance (identified by the name only).
//
// TODO(fss): this method should not be exported.
func (app *App) InstanceEnv(name string) map[string]bind.EnvVar {
	envs := make(map[string]bind.EnvVar)
	for k, env := range app.Env {
		if env.InstanceName == name {
			envs[k] = bind.EnvVar(env)
		}
	}
	return envs
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (app *App) Run(cmd string, w io.Writer, once bool) error {
	if !app.Available() {
		return stderr.New("App must be available to run commands")
	}
	app.Log(fmt.Sprintf("running '%s'", cmd), "tsuru")
	return app.sourced(cmd, w, once)
}

func (app *App) sourced(cmd string, w io.Writer, once bool) error {
	var mapEnv = func(name string) string {
		if e, ok := app.Env[name]; ok {
			return e.Value
		}
		if e := os.Getenv(name); e != "" {
			return e
		}
		return "${" + name + "}"
	}
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := "[ -d /home/application/current ] && cd /home/application/current"
	cmd = fmt.Sprintf("%s; %s; %s", source, cd, os.Expand(cmd, mapEnv))
	return app.run(cmd, w, once)
}

func (app *App) run(cmd string, w io.Writer, once bool) error {
	if once {
		return Provisioner.ExecuteCommandOnce(w, w, app, cmd)
	}
	return Provisioner.ExecuteCommand(w, w, app, cmd)
}

// Restart runs the restart hook for the app, writing its output to w.
func (app *App) Restart(w io.Writer) error {
	err := app.hookRunner().Restart(app, w, "before")
	if err != nil {
		return err
	}
	err = log.Write(w, []byte("\n ---> Restarting your app\n"))
	if err != nil {
		log.Errorf("[restart] error on write app log for the app %s - %s", app.Name, err)
		return err
	}
	err = Provisioner.Restart(app)
	if err != nil {
		log.Errorf("[restart] error on restart the app %s - %s", app.Name, err)
		return err
	}
	return app.hookRunner().Restart(app, w, "after")
}

func (app *App) hookRunner() hookRunner {
	if app.hr == nil {
		app.hr = &yamlHookRunner{}
	}
	return app.hr
}

func (app *App) Stop(w io.Writer) error {
	log.Write(w, []byte("\n ---> Stopping your app\n"))
	err := Provisioner.Stop(app)
	if err != nil {
		log.Errorf("[stop] error on stop the app %s - %s", app.Name, err)
		return err
	}
	units := make([]Unit, len(app.Units))
	for i, u := range app.Units {
		u.State = provision.StatusStopped.String()
		units[i] = u
	}
	app.Units = units
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(bson.M{"name": app.Name}, app)
}

func (app *App) Ready() error {
	app.State = "ready"
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"state": "ready"}})
}

// GetUnits returns the internal list of units converted to bind.Unit.
func (app *App) GetUnits() []bind.Unit {
	var units []bind.Unit
	for _, unit := range app.Units {
		copy := unit
		copy.app = app
		units = append(units, &copy)
	}
	return units
}

// GetName returns the name of the app.
func (app *App) GetName() string {
	return app.Name
}

// GetMemory returns the memory limit (in MB) for the app.
func (app *App) GetMemory() int {
	return app.Memory
}

// GetSwap returns the swap limit (in MB) for the app.
func (app *App) GetSwap() int {
	return app.Swap
}

// GetIp returns the ip of the app.
func (app *App) GetIp() string {
	return app.Ip
}

// GetPlatform returns the platform of the app.
func (app *App) GetPlatform() string {
	return app.Platform
}

func (app *App) GetDeploys() uint {
	return app.Deploys
}

type Deploy struct {
	App       string
	Timestamp time.Time
	Duration  time.Duration
	Commit    string
}

func (app *App) ListDeploys() ([]Deploy, error) {
	return listDeploys(app, nil)
}

func ListDeploys(s *service.Service) ([]Deploy, error) {
	return listDeploys(nil, s)
}

func listDeploys(app *App, s *service.Service) ([]Deploy, error) {
	var list []Deploy
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var qr bson.M
	if app != nil {
		qr = bson.M{"app": app.Name}
	}
	if s != nil {
		var instances []service.ServiceInstance
		q := bson.M{"service_name": s.Name}
		err = conn.ServiceInstances().Find(q).All(&instances)
		if err != nil {
			return nil, err
		}
		var appNames []string
		for _, instance := range instances {
			for _, apps := range instance.Apps {
				appNames = append(appNames, apps)
			}
		}
		qr = bson.M{"app": bson.M{"$in": appNames}}
	}
	if err := conn.Deploys().Find(qr).Sort("-timestamp").All(&list); err != nil {
		return nil, err
	}
	return list, err
}

// ProvisionedUnits returns the internal list of units converted to
// provision.AppUnit.
func (app *App) ProvisionedUnits() []provision.AppUnit {
	units := make([]provision.AppUnit, len(app.Units))
	for i, u := range app.Units {
		other := u
		other.app = app
		units[i] = &other
	}
	return units
}

// Env returns app.Env
func (app *App) Envs() map[string]bind.EnvVar {
	return app.Env
}

// SerializeEnvVars serializes the environment variables of the app. The
// environment variables will be written the the file /home/application/apprc
// in all units of the app.
func (app *App) SerializeEnvVars() error {
	var buf bytes.Buffer
	cmd := "cat > /home/application/apprc <<END\n"
	cmd += fmt.Sprintf("# generated by tsuru at %s\n", time.Now().Format(time.RFC822Z))
	for k, v := range app.Env {
		cmd += fmt.Sprintf(`export %s="%s"`+"\n", k, v.Value)
	}
	cmd += "END\n"
	err := app.run(cmd, &buf, false)
	if err != nil {
		output := buf.Bytes()
		if output == nil {
			err = fmt.Errorf("Failed to write env vars: %s.", err)
		} else {
			err = fmt.Errorf("Failed to write env vars (%s): %s.", err, output)
		}
	}
	return err
}

// SetEnvs saves a list of environment variables in the app. The publicOnly
// parameter indicates whether only public variables can be overridden (if set
// to false, SetEnvs may override a private variable).
func (app *App) SetEnvs(envs []bind.EnvVar, publicOnly bool) error {
	return app.setEnvsToApp(envs, publicOnly, false)
}

// setEnvsToApp adds environment variables to an app, serializing the resulting
// list of environment variables in all units of apps. This method can
// serialize them directly or using a queue.
//
// Besides the slice of environment variables, this method also takes two other
// parameters: publicOnly indicates whether only public variables can be
// overridden (if set to false, setEnvsToApp may override a private variable).
//
// If useQueue is true, it will use a queue to write the environment variables
// in the units of the app.
func (app *App) setEnvsToApp(envs []bind.EnvVar, publicOnly, useQueue bool) error {
	if len(envs) > 0 {
		for _, env := range envs {
			set := true
			if publicOnly {
				e, err := app.getEnv(env.Name)
				if err == nil && !e.Public {
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
		if useQueue {
			Enqueue(queue.Message{Action: regenerateApprc, Args: []string{app.Name}})
			return nil
		}
		go app.SerializeEnvVars()
	}
	return nil
}

// UnsetEnvs removes environment variables from an app, serializing the
// remaining list of environment variables to all units of the app.
//
// Besides the slice with the name of the variables, this method also takes the
// parameter publicOnly, which indicates whether only public variables can be
// overridden (if set to false, setEnvsToApp may override a private variable).
func (app *App) UnsetEnvs(variableNames []string, publicOnly bool) error {
	if len(variableNames) > 0 {
		for _, name := range variableNames {
			var unset bool
			e, err := app.getEnv(name)
			if !publicOnly || (err == nil && e.Public) {
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
		go app.SerializeEnvVars()
	}
	return nil
}

// SetCName defines the CName of the app. It updates the attribute,
// calls the SetCName function on the provisioner and saves
// the app in the database, returning an error when it cannot save the change
// in the database or set the CName on the provisioner.
func (app *App) SetCName(cname string) error {
	if cname != "" && !cnameRegexp.MatchString(cname) {
		return stderr.New("Invalid cname")
	}
	if s, ok := Provisioner.(provision.CNameManager); ok {
		if err := s.SetCName(app, cname); err != nil {
			return err
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app.CName = cname
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"cname": app.CName}},
	)
}

func (app *App) UnsetCName() error {
	if s, ok := Provisioner.(provision.CNameManager); ok {
		if err := s.UnsetCName(app, app.CName); err != nil {
			return err
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app.CName = ""
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"cname": app.CName}},
	)
}

// Log adds a log message to the app. Specifying a good source is good so the
// user can filter where the message come from.
func (app *App) Log(message, source string) error {
	messages := strings.Split(message, "\n")
	logs := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		if msg != "" {
			l := Applog{
				Date:    time.Now().In(time.UTC),
				Message: msg,
				Source:  source,
				AppName: app.Name,
			}
			logs = append(logs, l)
		}
	}
	if len(logs) > 0 {
		go notify(app.Name, logs)
		conn, err := db.Conn()
		if err != nil {
			return err
		}
		defer conn.Close()
		return conn.Logs().Insert(logs...)
	}
	return nil
}

// LastLogs returns a list of the last `lines` log of the app, matching the
// given source.
func (app *App) LastLogs(lines int, source string) ([]Applog, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	logs := []Applog{}
	q := bson.M{"appname": app.Name}
	if source != "" {
		q["source"] = source
	}
	err = conn.Logs().Find(q).Sort("-date").Limit(lines).All(&logs)
	if err != nil {
		return nil, err
	}
	l := len(logs)
	for i := 0; i < l/2; i++ {
		logs[i], logs[l-1-i] = logs[l-1-i], logs[i]
	}
	return logs, nil
}

// List returns the list of apps that the given user has access to.
//
// If the user does not have acces to any app, this function returns an empty
// list and a nil error.
func List(u *auth.User) ([]App, error) {
	var apps []App
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if u.IsAdmin() {
		if err := conn.Apps().Find(nil).All(&apps); err != nil {
			return []App{}, err
		}
		return apps, nil
	}
	ts, err := u.Teams()
	if err != nil {
		return []App{}, err
	}
	teams := auth.GetTeamsNames(ts)
	if err := conn.Apps().Find(bson.M{"teams": bson.M{"$in": teams}}).All(&apps); err != nil {
		return []App{}, err
	}
	return apps, nil
}

// Swap calls the Provisioner.Swap.
// And updates the app.CName in the database.
func Swap(app1, app2 *App) error {
	err := Provisioner.Swap(app1, app2)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app1.CName, app2.CName = app2.CName, app1.CName
	updateCName := func(app *App) error {
		return conn.Apps().Update(
			bson.M{"name": app.Name},
			bson.M{"$set": bson.M{"cname": app.CName}},
		)
	}
	err = updateCName(app1)
	if err != nil {
		return err
	}
	return updateCName(app2)
}

// DeployApp calls the Provisioner.Deploy
func DeployApp(app *App, version, commit string, writer io.Writer) error {
	start := time.Now()
	pipeline := Provisioner.DeployPipeline()
	if pipeline == nil {
		actions := []*action.Action{&ProvisionerDeploy, &IncrementDeploy}
		pipeline = action.NewPipeline(actions...)
	}
	logWriter := LogWriter{App: app, Writer: writer}
	err := pipeline.Execute(app, version, &logWriter)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	return saveDeployData(app.Name, commit, elapsed)
}

func saveDeployData(appName, commit string, duration time.Duration) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	deploy := Deploy{
		App:       appName,
		Timestamp: time.Now(),
		Duration:  duration,
		Commit:    commit,
	}
	return conn.Deploys().Insert(deploy)
}

func incrementDeploy(app *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$inc": bson.M{"deploys": 1}},
	)
}

// Start starts the app.
func (app *App) Start(w io.Writer) error {
	return Provisioner.Start(app)
}
