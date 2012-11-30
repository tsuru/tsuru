// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/juju"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"io"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type App struct {
	Env       map[string]bind.EnvVar
	Framework string
	Logs      []applog
	Name      string
	State     string
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
	return json.Marshal(&result)
}

type applog struct {
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

// createApp creates a new app.
//
// Creating a new app is a process composed of three steps:
//
//       1. Save the app in the database
//       2. Create S3 credentials and bucket for the app
//       3. Deploy juju charm
func createApp(a *App) error {
	a.State = "pending"
	err := db.Session.Apps().Insert(a)
	if err != nil {
		return err
	}
	env, err := createBucket(a)
	if err != nil {
		db.Session.Apps().Remove(bson.M{"name": a.Name})
		return err
	}
	host, _ := config.GetString("host")
	envVars := []bind.EnvVar{
		{Name: "APPNAME", Value: a.Name, Public: false, InstanceName: ""},
		{Name: "TSURU_HOST", Value: host, Public: false, InstanceName: ""},
	}
	variables := map[string]string{
		"ENDPOINT":           env.endpoint,
		"LOCATIONCONSTRAINT": strconv.FormatBool(env.locationConstraint),
		"ACCESS_KEY_ID":      env.AccessKey,
		"SECRET_KEY":         env.SecretKey,
		"BUCKET":             env.bucket,
	}
	for name, value := range variables {
		envVars = append(envVars, bind.EnvVar{
			Name:         fmt.Sprintf("TSURU_S3_%s", name),
			Value:        value,
			Public:       false,
			InstanceName: s3InstanceName,
		})
	}
	setEnvsToApp(a, envVars, false)
	return deploy(a)
}

// Deploys an app.
func deploy(a *App) error {
	a.log(fmt.Sprintf("creating app %s", a.Name), "tsuru")
	cmd := exec.Command("juju", "deploy", "--repository=/home/charms", "local:"+a.Framework, a.Name)
	log.Printf("deploying %s with name %s", a.Framework, a.Name)
	out, err := cmd.CombinedOutput()
	outStr := fmt.Sprintf("Failed to deploy: %s\n%s", err, out)
	a.log(outStr, "tsuru")
	log.Printf("executing %s", outStr)
	if err != nil {
		a.log(fmt.Sprintf("juju finished with exit status: %s", err), "tsuru")
		db.Session.Apps().Remove(bson.M{"name": a.Name})
		return errors.New(outStr)
	}
	return nil
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
		err = instance.Unbind(a)
		if err != nil {
			addMsg(instance.Name, err)
		}
	}
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (a *App) destroy() error {
	err := destroyBucket(a)
	if err != nil {
		return err
	}
	if len(a.Units) > 0 {
		out, err := a.unit().destroy()
		msg := fmt.Sprintf("Failed to destroy unit: %s\n%s", err, out)
		log.Print(msg)
		if err != nil {
			return errors.New(msg)
		}
		err = a.unbind()
		if err != nil {
			return err
		}
	}
	return db.Session.Apps().Remove(bson.M{"name": a.Name})
}

func (a *App) AddUnit(u *Unit) {
	for i, unt := range a.Units {
		if unt.Machine == u.Machine {
			a.Units[i] = *u
			return
		}
	}
	a.Units = append(a.Units, *u)
}

func (a *App) find(team *auth.Team) (int, bool) {
	pos := sort.Search(len(a.Teams), func(i int) bool {
		return a.Teams[i] >= team.Name
	})
	return pos, pos < len(a.Teams) && a.Teams[pos] == team.Name
}

func (a *App) grant(team *auth.Team) error {
	pos, found := a.find(team)
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

func (a *App) revoke(team *auth.Team) error {
	index, found := a.find(team)
	if !found {
		return errors.New("This team does not have access to this app")
	}
	copy(a.Teams[index:], a.Teams[index+1:])
	a.Teams = a.Teams[:len(a.Teams)-1]
	return nil
}

func (a *App) teams() []auth.Team {
	var teams []auth.Team
	db.Session.Teams().Find(bson.M{"_id": bson.M{"$in": a.Teams}}).All(&teams)
	return teams
}

func (a *App) setTeams(teams []auth.Team) {
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
	a.log(fmt.Sprintf("setting env %s with value %s", env.Name, env.Value), "tsuru")
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
		a.log(fmt.Sprintf("Got error while getting repository path: %s", err), "tsuru")
		return err
	}
	cmd := "cat " + path.Join(uRepo, "app.conf")
	var buf bytes.Buffer
	err = a.unit().Command(&buf, &buf, cmd)
	if err != nil {
		a.log(fmt.Sprintf("Got error while executing command: %s... Skipping hooks execution", err), "tsuru")
		return nil
	}
	err = goyaml.Unmarshal(juju.FilterOutput(buf.Bytes()), a.hooks)
	if err != nil {
		a.log(fmt.Sprintf("Got error while parsing yaml: %s", err), "tsuru")
		return err
	}
	return nil
}

func (a *App) runHook(w io.Writer, cmds []string, kind string) error {
	if len(cmds) == 0 {
		a.log(fmt.Sprintf("Skipping %s hooks...", kind), "tsuru")
		return nil
	}
	a.log(fmt.Sprintf("Executing %s hook...", kind), "tsuru")
	err := write(w, []byte("\n ---> Running "+kind+"\n"))
	if err != nil {
		return err
	}
	for _, cmd := range cmds {
		p, err := deployHookAbsPath(cmd)
		if err != nil {
			a.log(fmt.Sprintf("Error obtaining absolute path to hook: %s.", err), "tsuru")
			continue
		}
		err = a.run(p, w)
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

// run executes the command in app units
func (a *App) run(cmd string, w io.Writer) error {
	a.log(fmt.Sprintf("running '%s'", cmd), "tsuru")
	cmd = fmt.Sprintf("[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; %s", cmd)
	return a.unit().Command(w, w, cmd)
}

// restart runs the restart hook for the app
// and returns your output.
func restart(a *App, w io.Writer) error {
	u := a.unit()
	a.log("executing hook to restart", "tsuru")
	err := a.preRestart(w)
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Restarting your app\n"))
	if err != nil {
		return err
	}
	err = a.posRestart(w)
	if err != nil {
		return err
	}
	return u.executeHook("restart", w)
}

// installDeps runs the dependencies hook for the app
// and returns your output.
func installDeps(a *App, w io.Writer) error {
	a.log("executing hook dependencies", "tsuru")
	return a.unit().executeHook("dependencies", w)
}

func (a *App) unit() *Unit {
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

func (a *App) SetEnvs(envs []bind.EnvVar, publicOnly bool) error {
	e := make([]bind.EnvVar, len(envs))
	for i, env := range envs {
		e[i] = bind.EnvVar(env)
	}
	return setEnvsToApp(a, e, publicOnly)
}

func (a *App) UnsetEnvs(envs []string, publicOnly bool) error {
	return unsetEnvFromApp(a, envs, publicOnly)
}

func (a *App) log(message string, source string) error {
	log.Printf(message)
	messages := strings.Split(message, "\n")
	for _, msg := range messages {
		if msg != "" {
			l := applog{
				Date:    time.Now(),
				Message: msg,
				Source:  source,
			}
			a.Logs = append(a.Logs, l)
		}
	}
	return db.Session.Apps().Update(bson.M{"name": a.Name}, a)
}
