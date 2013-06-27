// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	stderr "errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/quota"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/service"
	"io"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os"
	"path"
	"regexp"
	"sort"
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
	Env      map[string]bind.EnvVar
	Platform string `bson:"framework"`
	Name     string
	Ip       string
	CName    string
	Units    []Unit
	Teams    []string
	Owner    string
	State    string
	conf     *conf
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
	return json.Marshal(&result)
}

// Applog represents a log entry.
type Applog struct {
	Date    time.Time
	Message string
	Source  string
	AppName string
}

type hooks struct {
	PreRestart  []string `yaml:"pre-restart"`
	PostRestart []string `yaml:"post-restart"`
}

type conf struct {
	Hooks hooks
}

// Get queries the database and fills the App object with data retrieved from
// the database. It uses the name of the app as filter in the query, so you can
// provide this field:
//
//     app := App{Name: "myapp"}
//     err := app.Get()
//     // do something with the app
func (app *App) Get() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Find(bson.M{"name": app.Name}).One(app)
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of four steps:
//
//       1. Save the app in the database
//       2. Create IAM credentials for the app
//       3. Create S3 bucket for the app (if the bucket support is enabled)
//       4. Create the git repository using gandalf
//       5. Provision units within the provisioner
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
	app.SetTeams(teams)
	app.Owner = user.Email
	if !app.isValid() {
		msg := "Invalid app name, your app should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."
		return &errors.ValidationError{Message: msg}
	}
	actions := []*action.Action{&reserveUserApp, &createAppQuota, &insertApp}
	useS3, _ := config.GetBool("bucket-support")
	if useS3 {
		actions = append(actions, &createIAMUserAction,
			&createIAMAccessKeyAction,
			&createBucketAction, &createUserPolicyAction)
	}
	actions = append(actions, &exportEnvironmentsAction,
		&createRepository, &provisionApp)
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

// ForceDestroy destroys an app with force.
//
// Destroying an app is a process composed of four steps:
//
//       1. Destroy the bucket and S3 credentials (if bucket-support is
//       enabled).
//       2. Destroy the app unit using juju
//       3. Unbind all service instances from the app
//       4. Remove the app from the database
func ForceDestroy(app *App) error {
	gURL := repository.ServerURL()
	(&gandalf.Client{Endpoint: gURL}).RemoveRepository(app.Name)
	useS3, _ := config.GetBool("bucket-support")
	if useS3 {
		destroyBucket(app)
	}
	if len(app.Units) > 0 {
		Provisioner.Destroy(app)
		app.unbind()
	}
	token := app.Env["TSURU_APP_TOKEN"].Value
	auth.DeleteToken(token)
	quota.Release(app.Owner, app.Name)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	quota.Delete(app.Name)
	return conn.Apps().Remove(bson.M{"name": app.Name})
}

// AddUnit adds a new unit to the app (or update an existing unit). It just updates
// the internal list of units, it does not talk to the provisioner. For
// provisioning a new unit for the app, one should use AddUnits method, which
// receives the number of units that you want to provision.
func (app *App) AddUnit(u *Unit) {
	for i, unt := range app.Units {
		if unt.Name == u.Name {
			u.QuotaItem = unt.QuotaItem
			app.Units[i] = *u
			return
		} else if unt.Name == "" && unt.QuotaItem == app.Name+"-0" {
			u.QuotaItem = unt.QuotaItem
			app.Units[i] = *u
			return
		}
	}
	u.QuotaItem = generateUnitQuotaItems(app, 1)[0]
	app.Units = append(app.Units, *u)
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (app *App) AddUnits(n uint) error {
	if n == 0 {
		return stderr.New("Cannot add zero units.")
	}
	return action.NewPipeline(
		&reserveUnitsToAdd,
		&provisionAddUnits,
		&saveNewUnitsInDatabase,
	).Execute(app, n)
}

// RemoveUnit removes a unit by its InstanceId or Name.
//
// Will search first by InstanceId, if no instance is found, then tries to
// search using the unit name, then calls the removal function from provisioner
//
// Returns an error in case of failure.
func (app *App) RemoveUnit(id string) error {
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
		return stderr.New("Unit not found.")
	}
	if err := Provisioner.RemoveUnit(app, unit.GetName()); err != nil {
		return err
	}
	app.removeUnits([]int{i})
	app.unbindUnit(&unit)
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
	items := make([]string, int(n))
	for i := 0; i < int(n); i++ {
		err = Provisioner.RemoveUnit(app, units[i].GetName())
		if err == nil {
			removed = append(removed, i)
		}
		app.unbindUnit(&units[i])
		items[i] = units[i].QuotaItem
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
		bson.M{"$set": bson.M{"units": app.Units}},
	)
	quota.Release(app.Name, items...)
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
		err = instance.UnbindUnit(unit)
		if err != nil {
			log.Printf("Error unbinding the unit %s with the service instance %s.", unit.GetIp(), instance.Name)
		}
	}
	return nil
}

// Available returns true if at least one of N units is started.
func (app *App) Available() bool {
	for _, unit := range app.ProvisionUnits() {
		if unit.GetStatus() == provision.StatusStarted {
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
		log.Printf("Failed to connect to the database: %s", err)
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

// loadConf loads app configuration from app.yaml.
func (app *App) loadConf() error {
	if app.conf != nil {
		return nil
	}
	app.conf = new(conf)
	uRepo, err := repository.GetPath()
	if err != nil {
		app.Log(fmt.Sprintf("Got error while getting repository path: %s", err), "tsuru")
		return err
	}
	cmd := "cat " + path.Join(uRepo, "app.yaml")
	var outStream, errStream bytes.Buffer
	if err := Provisioner.ExecuteCommand(&outStream, &errStream, app, cmd); err != nil {
		return nil
	}
	err = goyaml.Unmarshal(outStream.Bytes(), app.conf)
	if err != nil {
		app.Log(fmt.Sprintf("Got error while parsing yaml: %s", err), "tsuru")
		return err
	}
	return nil
}

// preRestart is responsible for running user's pre-restart script.
//
// The path to this script can be found at the app.conf file, at the root of user's app repository.
func (app *App) preRestart(w io.Writer) error {
	if err := app.loadConf(); err != nil {
		return err
	}
	return app.runHook(w, app.conf.Hooks.PreRestart, "pre-restart")
}

// posRestart is responsible for running user's post-restart script.
//
// The path to this script can be found at the app.conf file, at the root of
// user's app repository.
func (app *App) postRestart(w io.Writer) error {
	if err := app.loadConf(); err != nil {
		return err
	}
	return app.runHook(w, app.conf.Hooks.PostRestart, "post-restart")
}

// runHook executes the given list of commands, as a hook identified by the
// kind string. If the list is empty, it returns nil.
//
// The hook itself may be "pre-restart" or "post-restart".
func (app *App) runHook(w io.Writer, cmds []string, kind string) error {
	if len(cmds) == 0 {
		return nil
	}
	app.Log(fmt.Sprintf("Executing %s hook...", kind), "tsuru")
	err := log.Write(w, []byte("\n ---> Running "+kind+"\n"))
	if err != nil {
		return err
	}
	for _, cmd := range cmds {
		err = app.sourced(cmd, w)
		if err != nil {
			return err
		}
	}
	return err
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (app *App) Run(cmd string, w io.Writer) error {
	if !app.Available() {
		return stderr.New("App must be available to run commands")
	}
	app.Log(fmt.Sprintf("running '%s'", cmd), "tsuru")
	return app.sourced(cmd, w)
}

func (app *App) sourced(cmd string, w io.Writer) error {
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
	return app.run(cmd, w)
}

func (app *App) run(cmd string, w io.Writer) error {
	return Provisioner.ExecuteCommand(w, w, app, cmd)
}

// Restart runs the restart hook for the app, writing its output to w.
func (app *App) Restart(w io.Writer) error {
	app.Log("executing hook to restart", "tsuru")
	err := app.preRestart(w)
	if err != nil {
		return err
	}
	err = log.Write(w, []byte("\n ---> Restarting your app\n"))
	if err != nil {
		return err
	}
	err = Provisioner.Restart(app)
	if err != nil {
		return err
	}
	return app.postRestart(w)
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
	for _, u := range app.Units {
		u.app = app
		units = append(units, &u)
	}
	return units
}

// GetName returns the name of the app.
func (app *App) GetName() string {
	return app.Name
}

// GetIp returns the ip of the app.
func (app *App) GetIp() string {
	return app.Ip
}

// GetPlatform returns the platform of the app.
func (app *App) GetPlatform() string {
	return app.Platform
}

// ProvisionUnits returns the internal list of units converted to
// provision.AppUnit.
func (app *App) ProvisionUnits() []provision.AppUnit {
	units := make([]provision.AppUnit, len(app.Units))
	for i, u := range app.Units {
		other := u
		other.app = app
		units[i] = &other
	}
	return units
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
	err := app.run(cmd, &buf)
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
	var logs []Applog
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
