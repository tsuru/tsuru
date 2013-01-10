// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
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

func write(w io.Writer, content []byte) error {
	n, err := w.Write(content)
	if err != nil {
		return err
	}
	if n != len(content) {
		return io.ErrShortWrite
	}
	return nil
}

type App struct {
	Env       map[string]bind.EnvVar
	Framework string
	Logs      []Applog
	Name      string
	State     string
	Ip        string
	Units     []Unit
	Teams     []string
	hooks     *conf
}

func (a *App) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})
	result["Name"] = a.Name
	result["State"] = a.State
	result["Framework"] = a.Framework
	result["Teams"] = a.Teams
	result["Units"] = a.Units
	result["Repository"] = repository.GetUrl(a.Name)
	result["Ip"] = a.Ip
	return json.Marshal(&result)
}

type Applog struct {
	Date    time.Time
	Message string
	Source  string
}

type conf struct {
	PreRestart []string `yaml:"pre-restart"`
	PosRestart []string `yaml:"pos-restart"`
}

func (a *App) Get() error {
	return db.Session.Apps().Find(bson.M{"name": a.Name}).One(a)
}

// CreateApp creates a new app.
//
// Creating a new app is a process composed of four steps:
//
//       1. Save the app in the database
//       2. Create IAM credentials for the app
//       3. Create S3 bucket for the app
//       4. Create the git repository using gandalf
//       5. Provision units within the provisioner
func CreateApp(a *App, units uint) error {
	if units == 0 {
		return &ValidationError{Message: "Cannot create app with 0 units."}
	}
	if !a.isValid() {
		msg := "Invalid app name, your app should have at most 63 " +
			"characters, containing only lower case letters or numbers, " +
			"starting with a letter."
		return &ValidationError{Message: msg}
	}
	pipeline := action.NewPipeline(
		&insertApp, &createIAMUserAction, &createIAMAccessKeyAction,
		&createBucketAction, &createUserPolicyAction,
		&exportEnvironmentsAction, &createRepository, &provisionApp,
		&provisionAddUnits,
	)
	return pipeline.Execute(a, units)
}

func (a *App) unbind() error {
	var instances []service.ServiceInstance
	err := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).All(&instances)
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
		err = instance.UnbindApp(a)
		if err != nil {
			addMsg(instance.Name, err)
		}
	}
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

// Destroy destroys an app.
//
// Destroy an app is a process composed of x steps:
//
//       1. Destroy the bucket and S3 credentials
//       2. Destroy the app unit using juju
//       3. Execute the unbind for the app
//       4. Remove the app from the database
func (a *App) Destroy() error {
	err := destroyBucket(a)
	if err != nil {
		return err
	}
	if len(a.Units) > 0 {
		err = Provisioner.Destroy(a)
		if err != nil {
			return errors.New("Failed to destroy the app: " + err.Error())
		}
		err = a.unbind()
		if err != nil {
			return err
		}
	}
	return db.Session.Apps().Remove(bson.M{"name": a.Name})
}

// AddUnit adds a new unit to the app (or update an existing unit). It just updates
// the internal list of units, it does not talk to the provisioner. For
// provisioning a new unit for the app, one should use AddUnits method, which
// receives the number of units that you want to provision.
func (a *App) AddUnit(u *Unit) {
	for i, unt := range a.Units {
		if unt.Name == u.Name {
			a.Units[i] = *u
			return
		}
	}
	a.Units = append(a.Units, *u)
}

// AddUnits creates n new units within the provisioner, saves new units in the
// database and enqueues the apprc serialization.
func (a *App) AddUnits(n uint) error {
	if n == 0 {
		return errors.New("Cannot add zero units.")
	}
	units, err := Provisioner.AddUnits(a, n)
	if err != nil {
		return err
	}
	qArgs := make([]string, len(units)+1)
	qArgs[0] = a.Name
	length := len(a.Units)
	appUnits := make([]Unit, len(units))
	a.Units = append(a.Units, appUnits...)
	messages := make([]queue.Message, len(units)*3)
	mCount := 0
	for i, unit := range units {
		a.Units[i+length] = Unit{
			Name:    unit.Name,
			Type:    unit.Type,
			Ip:      unit.Ip,
			Machine: unit.Machine,
			State:   provision.StatusPending.String(),
		}
		qArgs[i+1] = unit.Name
		messages[mCount] = queue.Message{Action: regenerateApprc, Args: []string{a.Name, unit.Name}}
		messages[mCount+1] = queue.Message{Action: startApp, Args: []string{a.Name, unit.Name}}
		messages[mCount+2] = queue.Message{Action: bindService, Args: []string{a.Name, unit.Name}}
		mCount += 3
	}
	err = db.Session.Apps().Update(bson.M{"name": a.Name}, a)
	if err != nil {
		return err
	}
	go enqueue(messages...)
	return nil
}

func (a *App) removeUnits(indices []int) {
	sequential := true
	for i := range indices {
		if i != indices[i] {
			sequential = false
			break
		}
	}
	if sequential {
		a.Units = a.Units[len(indices):]
	} else {
		for i, index := range indices {
			index -= i
			if index+1 < len(a.Units) {
				copy(a.Units[index:], a.Units[index+1:])
			}
			a.Units = a.Units[:len(a.Units)-1]
		}
	}
}

func (a *App) RemoveUnits(n uint) error {
	if n == 0 {
		return errors.New("Cannot remove zero units.")
	} else if l := uint(len(a.Units)); l == n {
		return errors.New("Cannot remove all units from an app.")
	} else if n > l {
		return fmt.Errorf("Cannot remove %d units from this app, it has only %d units.", n, l)
	}
	indices, err := Provisioner.RemoveUnits(a, n)
	if err != nil {
		return err
	}
	for _, i := range indices {
		unit := a.ProvisionUnits()[i]
		a.unbindUnit(unit)
	}
	a.removeUnits(indices)
	return db.Session.Apps().Update(bson.M{"name": a.Name}, a)
}

func (a *App) unbindUnit(unit provision.AppUnit) error {
	var instances []service.ServiceInstance
	err := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).All(&instances)
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

func (a *App) Find(team *auth.Team) (int, bool) {
	pos := sort.Search(len(a.Teams), func(i int) bool {
		return a.Teams[i] >= team.Name
	})
	return pos, pos < len(a.Teams) && a.Teams[pos] == team.Name
}

func (a *App) Grant(team *auth.Team) error {
	pos, found := a.Find(team)
	if found {
		return errors.New("This team already has access to this app")
	}
	a.Teams = append(a.Teams, "")
	tmp := a.Teams[pos]
	for i := pos; i < len(a.Teams)-1; i++ {
		a.Teams[i+1], tmp = tmp, a.Teams[i]
	}
	a.Teams[pos] = team.Name
	return nil
}

func (a *App) Revoke(team *auth.Team) error {
	index, found := a.Find(team)
	if !found {
		return errors.New("This team does not have access to this app")
	}
	copy(a.Teams[index:], a.Teams[index+1:])
	a.Teams = a.Teams[:len(a.Teams)-1]
	return nil
}

func (a *App) GetTeams() []auth.Team {
	var teams []auth.Team
	db.Session.Teams().Find(bson.M{"_id": bson.M{"$in": a.Teams}}).All(&teams)
	return teams
}

func (a *App) SetTeams(teams []auth.Team) {
	a.Teams = make([]string, len(teams))
	for i, team := range teams {
		a.Teams[i] = team.Name
	}
	sort.Strings(a.Teams)
}

func (a *App) setEnv(env bind.EnvVar) {
	if a.Env == nil {
		a.Env = make(map[string]bind.EnvVar)
	}
	a.Env[env.Name] = env
	a.Log(fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru")
}

func (a *App) getEnv(name string) (bind.EnvVar, error) {
	var (
		env bind.EnvVar
		err error
		ok  bool
	)
	if env, ok = a.Env[name]; !ok {
		err = errors.New("Environment variable not declared for this app.")
	}
	return env, err
}

func (a *App) isValid() bool {
	regex := regexp.MustCompile(`^[a-z][a-z0-9]{0,62}$`)
	return regex.MatchString(a.Name)
}

func (a *App) InstanceEnv(name string) map[string]bind.EnvVar {
	envs := make(map[string]bind.EnvVar)
	for k, env := range a.Env {
		if env.InstanceName == name {
			envs[k] = bind.EnvVar(env)
		}
	}
	return envs
}

func deployHookAbsPath(p string) (string, error) {
	repoPath, err := config.GetString("git:unit-repo")
	if err != nil {
		return "", nil
	}
	cmdArgs := strings.Fields(p)
	abs := path.Join(repoPath, cmdArgs[0])
	_, err = os.Stat(abs)
	if os.IsNotExist(err) {
		return p, nil
	}
	cmdArgs[0] = abs
	return strings.Join(cmdArgs, " "), nil
}

// Loads restart hooks from app.conf.
func (a *App) loadHooks() error {
	if a.hooks != nil {
		return nil
	}
	a.hooks = new(conf)
	uRepo, err := repository.GetPath()
	if err != nil {
		a.Log(fmt.Sprintf("Got error while getting repository path: %s", err), "tsuru")
		return err
	}
	cmd := "cat " + path.Join(uRepo, "app.conf")
	var buf bytes.Buffer
	err = a.run(cmd, &buf)
	if err != nil {
		a.Log(fmt.Sprintf("Got error while executing command: %s... Skipping hooks execution", err), "tsuru")
		return nil
	}
	err = goyaml.Unmarshal(buf.Bytes(), a.hooks)
	if err != nil {
		a.Log(fmt.Sprintf("Got error while parsing yaml: %s", err), "tsuru")
		return err
	}
	return nil
}

func (a *App) runHook(w io.Writer, cmds []string, kind string) error {
	if len(cmds) == 0 {
		a.Log(fmt.Sprintf("Skipping %s hooks...", kind), "tsuru")
		return nil
	}
	a.Log(fmt.Sprintf("Executing %s hook...", kind), "tsuru")
	err := write(w, []byte("\n ---> Running "+kind+"\n"))
	if err != nil {
		return err
	}
	for _, cmd := range cmds {
		p, err := deployHookAbsPath(cmd)
		if err != nil {
			a.Log(fmt.Sprintf("Error obtaining absolute path to hook: %s.", err), "tsuru")
			continue
		}
		err = a.Run(p, w)
		if err != nil {
			return err
		}
	}
	return err
}

// preRestart is responsible for running user's pre-restart script.
//
// The path to this script can be found at the app.conf file, at the root of user's app repository.
func (a *App) preRestart(w io.Writer) error {
	if err := a.loadHooks(); err != nil {
		return err
	}
	return a.runHook(w, a.hooks.PreRestart, "pre-restart")
}

// posRestart is responsible for running user's pos-restart script.
//
// The path to this script can be found at the app.conf file, at the root of
// user's app repository.
func (a *App) posRestart(w io.Writer) error {
	if err := a.loadHooks(); err != nil {
		return err
	}
	return a.runHook(w, a.hooks.PosRestart, "pos-restart")
}

// Run executes the command in app units, sourcing apprc before running the
// command.
func (a *App) Run(cmd string, w io.Writer) error {
	a.Log(fmt.Sprintf("running '%s'", cmd), "tsuru")
	source := "[ -f /home/application/apprc ] && source /home/application/apprc"
	cd := "[ -d /home/application/current ] && cd /home/application/current"
	cmd = fmt.Sprintf("%s; %s; %s", source, cd, cmd)
	return a.run(cmd, w)
}

func (a *App) run(cmd string, w io.Writer) error {
	if a.State != string(provision.StatusStarted) {
		return fmt.Errorf("App must be started to run commands, but it is %q.", a.State)
	}
	return Provisioner.ExecuteCommand(w, w, a, cmd)
}

// Command is declared just to satisfy repository.Unit interface.
func (a *App) Command(stdout, stderr io.Writer, cmdArgs ...string) error {
	return Provisioner.ExecuteCommand(stdout, stderr, a, cmdArgs[0], cmdArgs[1:]...)
}

// Restart runs the restart hook for the app
// and returns your output.
func (a *App) Restart(w io.Writer) error {
	a.Log("executing hook to restart", "tsuru")
	err := a.preRestart(w)
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Restarting your app\n"))
	if err != nil {
		return err
	}
	err = a.run("/var/lib/tsuru/hooks/restart", w)
	if err != nil {
		return err
	}
	return a.posRestart(w)
}

// InstallDeps runs the dependencies hook for the app
// and returns your output.
func (a *App) InstallDeps(w io.Writer) error {
	return a.run("/var/lib/tsuru/hooks/dependencies", w)
}

func (a *App) Unit() *Unit {
	if len(a.Units) > 0 {
		unit := a.Units[0]
		unit.app = a
		return &unit
	}
	return &Unit{app: a}
}

func (a *App) GetUnits() []bind.Unit {
	var units []bind.Unit
	for _, u := range a.Units {
		u.app = a
		units = append(units, &u)
	}
	return units
}

func (a *App) GetName() string {
	return a.Name
}

func (a *App) GetFramework() string {
	return a.Framework
}

func (a *App) ProvisionUnits() []provision.AppUnit {
	units := make([]provision.AppUnit, len(a.Units))
	for i, u := range a.Units {
		other := u
		other.app = a
		units[i] = &other
	}
	return units
}

// SerializeEnvVars serializes the environment variables of the app. The
// environment variables will be written the the file /home/application/apprc
// in all units of the app.
//
// The wait parameter indicates whether it should wait or not for the write to
// complete.
func (a *App) SerializeEnvVars() error {
	var buf bytes.Buffer
	cmd := "cat > /home/application/apprc <<END\n"
	cmd += fmt.Sprintf("# generated by tsuru at %s\n", time.Now().Format(time.RFC822Z))
	for k, v := range a.Env {
		cmd += fmt.Sprintf(`export %s="%s"`+"\n", k, v.Value)
	}
	cmd += "END\n"
	err := a.run(cmd, &buf)
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

func (a *App) SetEnvs(envs []bind.EnvVar, publicOnly bool) error {
	return a.SetEnvsToApp(envs, publicOnly, false)
}

// SetEnvsToApp adds environment variables to an app, serializing the resulting
// list of environment variables in all units of apps. This method can
// serialize them directly or using a queue.
//
// Besides the slice of environment variables, this method also takes two other
// parameters: publicOnly indicates whether only public variables can be
// overridden (if set to false, setEnvsToApp may override a private variable).
//
// If useQueue is true, it will use a queue to write the environment variables
// in the units of the app.
func (app *App) SetEnvsToApp(envs []bind.EnvVar, publicOnly, useQueue bool) error {
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
		if err := db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
			return err
		}
		if useQueue {
			enqueue(queue.Message{Action: regenerateApprc, Args: []string{app.Name}})
			return nil
		}
		go app.SerializeEnvVars()
	}
	return nil
}

// UnsetEnvs removes environment variables from an app, serializing the
// remaining list of environment variables to all units of the app. Unlike
// SetEnvsToApp method, this method does not provide an option to use a queue
// for serialization.
//
// Besides the slice with the name of the variables, this method also takes two
// other parameters: publicOnly indicates whether only public variables can be
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
		if err := db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
			return err
		}
		go app.SerializeEnvVars()
	}
	return nil
}

func (a *App) Log(message string, source string) error {
	log.Printf(message)
	messages := strings.Split(message, "\n")
	for _, msg := range messages {
		if msg != "" {
			l := Applog{
				Date:    time.Now(),
				Message: msg,
				Source:  source,
			}
			a.Logs = append(a.Logs, l)
		}
	}
	return db.Session.Apps().Update(bson.M{"name": a.Name}, a)
}

type ValidationError struct {
	Message string
}

func (err *ValidationError) Error() string {
	return err.Message
}

func List(u *auth.User) ([]App, error) {
	var apps []App
	if u.IsAdmin() {
		if err := db.Session.Apps().Find(nil).All(&apps); err != nil {
			return []App{}, err
		}
		return apps, nil
	}
	ts, err := u.Teams()
	if err != nil {
		return []App{}, err
	}
	teams := auth.GetTeamsNames(ts)
	if err := db.Session.Apps().Find(bson.M{"teams": bson.M{"$in": teams}}).All(&apps); err != nil {
		return []App{}, err
	}
	return apps, nil
}
