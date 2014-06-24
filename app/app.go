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
	"sync"
	"time"
)

var Provisioner provision.Provisioner
var AuthScheme auth.Scheme

var (
	nameRegexp  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	cnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][\w-.]+$`)
)

const (
	InternalAppName = "tsr"
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

// App is the main type in tsuru. An app represents a real world application.
// This struct holds information about the app: its name, address, list of
// teams that have access to it, used platform, etc.
type App struct {
	Env            map[string]bind.EnvVar
	Platform       string `bson:"framework"`
	Name           string
	Ip             string
	CName          string
	Teams          []string
	TeamOwner      string
	Owner          string
	State          string
	Deploys        uint
	Memory         int `json:",string"`
	Swap           int `json:",string"`
	UpdatePlatform bool
	Lock           AppLock

	quota.Quota
	hr hookRunner
}

// Units returns the ilist of units.
func (app *App) Units() []provision.Unit {
	return Provisioner.Units(app)
}

// MarshalJSON marshals the app in json format. It returns a JSON object with
// the following keys: name, framework, teams, units, repository and ip.
func (app *App) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})
	result["name"] = app.Name
	result["platform"] = app.Platform
	result["teams"] = app.Teams
	result["units"] = app.Units()
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
	Unit    string
}

// Acquire an application lock by setting the lock field in the database.
// This method is already called by a connection middleware on requests with
// :app or :appname params that have side-effects.
func AcquireApplicationLock(appName string, owner string, reason string) (bool, error) {
	conn, err := db.Conn()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	appLock := AppLock{
		Locked:      true,
		Reason:      reason,
		Owner:       owner,
		AcquireDate: time.Now().In(time.UTC),
	}
	err = conn.Apps().Update(bson.M{"name": appName, "lock.locked": bson.M{"$in": []interface{}{false, nil}}}, bson.M{"$set": bson.M{"lock": appLock}})
	if err == mgo.ErrNotFound {
		// TODO(cezarsa): Maybe handle lock expiring by checking timestamp
		return false, nil
	}
	return err == nil, err
}

// Releases a lock hold on an app, currently it's called by a middleware,
// however, ideally, it should be called individually by each handler since
// they might be doing operations in background.
func ReleaseApplicationLock(appName string) {
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Error getting DB, couldn't unlock %s: %s", appName, err.Error())
	}
	defer conn.Close()
	err = conn.Apps().Update(bson.M{"name": appName, "lock.locked": true}, bson.M{"$set": bson.M{"lock": AppLock{}}})
	if err != nil {
		log.Errorf("Error updating entry, couldn't unlock %s: %s", appName, err.Error())
	}
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
		&setAppIp,
	}
	pipeline := action.NewPipeline(actions...)
	err = pipeline.Execute(app, user)
	if err != nil {
		return &AppCreationError{app: app.Name, Err: err}
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

func asyncDestroyAppProvisioner(app *App) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := Provisioner.Destroy(app)
		if err != nil {
			log.Errorf("Unable to destroy app in provisioner: %s", err.Error())
		}
		err = app.unbind()
		if err != nil {
			log.Errorf("Unable to unbind app in provisioner: %s", err.Error())
		}
	}()
	return &wg
}

// Delete deletes an app.
//
// Delete an app is a process composed of three steps:
//
//       1. Destroy the app unit
//       2. Unbind all service instances from the app
//       3. Remove the app from the database
func Delete(app *App) error {
	appName := app.Name
	wg := asyncDestroyAppProvisioner(app)
	wg.Add(1)
	defer wg.Done()
	go func() {
		defer ReleaseApplicationLock(appName)
		wg.Wait()
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("Unable to delete app %s from db: %s", appName, err.Error())
		}
		defer conn.Close()
		err = conn.Logs(appName).DropCollection()
		if err != nil {
			log.Errorf("Ignored error dropping logs collection for app %s: %s", appName, err.Error())
		}
		err = conn.Apps().Remove(bson.M{"name": appName})
		if err != nil {
			log.Errorf("Error trying to destroy app %s from db: %s", appName, err.Error())
		}
	}()
	gandalfClient := gandalf.Client{Endpoint: repository.ServerURL()}
	gandalfClient.RemoveRepository(appName)
	token := app.Env["TSURU_APP_TOKEN"].Value
	err := AuthScheme.Logout(token)
	if err != nil {
		log.Errorf("Unable to remove app token in destroy: %s", err.Error())
	}
	owner, err := auth.GetUserByEmail(app.Owner)
	if err != nil {
		log.Errorf("Unable to get app owner in destroy: %s", err.Error())
	} else {
		err = auth.ReleaseApp(owner)
		if err != nil {
			log.Errorf("Unable to release app quota: %s", err.Error())
		}
	}
	return nil
}

// Add provisioned units to database and enqueue messages to
// bind services and regenerate apprc. It's called as one of
// the steps started by AddUnits(). It doesn't call the
// provisioner.
func (app *App) AddUnitsToDB(units []provision.Unit) error {
	messages := make([]queue.Message, len(units)*2)
	mCount := 0
	for _, unit := range units {
		messages[mCount] = queue.Message{Action: regenerateApprc, Args: []string{app.Name, unit.Name}}
		messages[mCount+1] = queue.Message{Action: BindService, Args: []string{app.Name, unit.Name}}
		mCount += 2
	}
	go Enqueue(messages...)
	return nil
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

// RemoveUnit removes a unit by its Name.
func (app *App) RemoveUnit(name string) error {
	unit, err := app.findUnitByName(name)
	if err != nil {
		return err
	}
	return Provisioner.RemoveUnit(app, unit.Name)
}

// findUnitByName searchs unit by name.
func (app *App) findUnitByName(name string) (*provision.Unit, error) {
	for _, u := range app.Units() {
		if u.Name == name {
			return &u, nil
		}
	}
	return nil, stderr.New(fmt.Sprintf("Unit not found: %s.", name))
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
	} else if l := uint(len(app.Units())); l == n {
		return stderr.New("Cannot remove all units from an app.")
	} else if n > l {
		return fmt.Errorf("Cannot remove %d units from this app, it has only %d units.", n, l)
	}
	var (
		removed []int
		err     error
	)
	units := UnitSlice(app.Units())
	sort.Sort(units)
	for i := 0; i < int(n); i++ {
		name := units[i].Name
		go Provisioner.RemoveUnit(app, name)
		removed = append(removed, i)
	}
	if len(removed) == 0 {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	dbErr := conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{
			"$set": bson.M{
				"quota.inuse": len(app.Units()),
			},
		},
	)
	if err == nil {
		return dbErr
	}
	return err
}

// Available returns true if at least one of N units is started or unreachable.
func (app *App) Available() bool {
	for _, unit := range app.Units() {
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
		err = stderr.New("Environment variable not declared for this app.")
	}
	return env, err
}

// isValid indicates whether the name of the app is valid.
func (app *App) isValid() bool {
	return app.Name != InternalAppName && nameRegexp.MatchString(app.Name)
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
	app.Log(fmt.Sprintf("running '%s'", cmd), "tsuru", "api")
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
	return nil
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
	for _, unit := range app.Units() {
		units = append(units, &unit)
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

type deploy struct {
	App       string
	Timestamp time.Time
	Duration  time.Duration
	Commit    string
	Error     string
}

func (app *App) ListDeploys() ([]deploy, error) {
	return listDeploys(app, nil)
}

func ListDeploys(s *service.Service) ([]deploy, error) {
	return listDeploys(nil, s)
}

func listDeploys(app *App, s *service.Service) ([]deploy, error) {
	var list []deploy
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
		return app.SerializeEnvVars()
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
		return app.SerializeEnvVars()
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
		conn, err := db.Conn()
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
	conn, err := db.Conn()
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
	err = conn.Logs(app.Name).Find(q).Sort("-_id").Limit(lines).All(&logs)
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

type DeployOptions struct {
	App          *App
	Version      string
	Commit       string
	ArchiveURL   string
	OutputStream io.Writer
}

// Deploy runs a deployment of an application. It will first try to run an
// archive based deploy (if opts.ArchiveURL is not empty), and then fallback to
// the Git based deployment.
func Deploy(opts DeployOptions) error {
	var pipeline *action.Pipeline
	start := time.Now()
	if cprovisioner, ok := Provisioner.(provision.CustomizedDeployPipelineProvisioner); ok {
		pipeline = cprovisioner.DeployPipeline()
	} else {
		actions := []*action.Action{&ProvisionerDeploy, &IncrementDeploy}
		pipeline = action.NewPipeline(actions...)
	}
	logWriter := LogWriter{App: opts.App, Writer: opts.OutputStream}
	err := pipeline.Execute(opts, &logWriter)
	elapsed := time.Since(start)
	if err != nil {
		saveDeployData(opts.App.Name, opts.Commit, elapsed, err)
		return err
	}
	if opts.App.UpdatePlatform == true {
		opts.App.SetUpdatePlatform(false)
	}
	return saveDeployData(opts.App.Name, opts.Commit, elapsed, nil)
}

func saveDeployData(appName, commit string, duration time.Duration, deployError error) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	deploy := deploy{
		App:       appName,
		Timestamp: time.Now(),
		Duration:  duration,
		Commit:    commit,
	}
	if deployError != nil {
		deploy.Error = deployError.Error()
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

// Start starts the app calling the provisioner.Start method and
// changing the units state to StatusStarted.
func (app *App) Start(w io.Writer) error {
	err := Provisioner.Start(app)
	if err != nil {
		log.Errorf("[start] error on start the app %s - %s", app.Name, err)
		return err
	}
	return nil
}

func (app *App) SetUpdatePlatform(check bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$set": bson.M{"updateplatform": check}},
	)
}

func (app *App) GetUpdatePlatform() bool {
	return app.UpdatePlatform
}
