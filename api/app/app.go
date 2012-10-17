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
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"io"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"
)

const confSep = "========"

type App struct {
	Env       map[string]bind.EnvVar
	Framework string
	Logs      []applog
	Name      string
	State     string
	Units     []Unit
	Teams     []string
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
// Creating a new app is a process composed of two steps:
//
//       1. Saves the app in the database
//       2. Deploys juju charm
func createApp(a *App) error {
	a.State = "pending"
	err := db.Session.Apps().Insert(a)
	if err != nil {
		return err
	}
	return deploy(a)
}

// Deploys an app.
func deploy(a *App) error {
	a.log(fmt.Sprintf("creating app %s", a.Name))
	cmd := exec.Command("juju", "deploy", "--repository=/home/charms", "local:"+a.Framework, a.Name)
	log.Printf("deploying %s with name %s", a.Framework, a.Name)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	a.log(outStr)
	log.Printf("executing %s", outStr)
	if err != nil {
		a.log(fmt.Sprintf("juju finished with exit status: %s", err))
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
	out, err := a.unit().destroy()
	msg := string(out)
	log.Print(msg)
	if err != nil {
		return errors.New(msg)
	}
	err = a.unbind()
	if err != nil {
		return err
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
	a.log(fmt.Sprintf("setting env %s with value %s", env.Name, env.Value))
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

// Returns app.conf located at app's git repository
func (a *App) conf() (conf, error) {
	var c conf
	uRepo, err := repository.GetPath()
	if err != nil {
		a.log(fmt.Sprintf("Got error while getting repository path: %s", err))
		return c, err
	}
	cPath := path.Join(uRepo, "app.conf")
	cmd := fmt.Sprintf(`echo "%s";cat %s`, confSep, cPath)
	var buf bytes.Buffer
	err = a.unit().Command(&buf, &buf, cmd)
	if err != nil {
		a.log(fmt.Sprintf("Got error while executing command: %s... Skipping hooks execution", err))
		return c, nil
	}
	out := buf.String()
	data := strings.Split(out, confSep)[1]
	err = goyaml.Unmarshal([]byte(data), &c)
	if err != nil {
		a.log(fmt.Sprintf("Got error while parsing yaml: %s", err))
		return c, err
	}
	return c, nil
}

func (a *App) runHook(cmds []string, kind string) ([]byte, error) {
	var (
		buf bytes.Buffer
		err error
	)
	a.log(fmt.Sprintf("Executing %s hook...", kind))
	for _, cmd := range cmds {
		p, err := deployHookAbsPath(cmd)
		if err != nil {
			a.log(fmt.Sprintf("Error obtaining absolute path to hook: %s.", err))
			continue
		}
		err = a.run(p, &buf)
		if err != nil {
			return nil, err
		}
	}
	a.log(fmt.Sprintf("Output of %s hooks: %s", kind, buf.Bytes()))
	return buf.Bytes(), err
}

// preRestart is responsible for running user's pre-restart script.
//
// The path to this script can be found at the app.conf file, at the root of user's app repository.
func (a *App) preRestart(c conf) ([]byte, error) {
	if !a.hasRestartHooks(c) {
		a.log("app.conf file does not exists or is in the right place. Skipping pre-restart hook...")
		return []byte(nil), nil
	}
	if len(c.PreRestart) == 0 {
		a.log("pre-restart hook section in app conf does not exists... Skipping pre-restart hook...")
		return []byte(nil), nil
	}
	return a.runHook(c.PreRestart, "pre-restart")
}

// posRestart is responsible for running user's pos-restart script.
//
// The path to this script can be found at the app.conf file, at the root of user's app repository.
func (a *App) posRestart(c conf) ([]byte, error) {
	if !a.hasRestartHooks(c) {
		a.log("app.conf file does not exists or is in the right place. Skipping pos-restart hook...")
		return []byte(nil), nil
	}
	if len(c.PosRestart) == 0 {
		a.log("pos-restart hook section in app conf does not exists... Skipping pos-restart hook...")
		return []byte(nil), nil
	}
	return a.runHook(c.PosRestart, "pos-restart")
}

func (a *App) hasRestartHooks(c conf) bool {
	return len(c.PreRestart) > 0 || len(c.PosRestart) > 0
}

// run executes the command in app units
func (a *App) run(cmd string, w io.Writer) error {
	a.log(fmt.Sprintf("running '%s'", cmd))
	cmd = fmt.Sprintf("[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; %s", cmd)
	return a.unit().Command(w, w, cmd)
}

// restart runs the restart hook for the app
// and returns your output.
func restart(a *App, w io.Writer) error {
	u := a.unit()
	a.log("executing hook to restart")
	conf, _ := a.conf()
	err := write(w, []byte("\n ---> Running pre-restart\n"))
	if err != nil {
		return err
	}
	a.preRestart(conf)
	err = write(w, []byte("\n ---> Restarting your app\n"))
	if err != nil {
		return err
	}
	return u.executeHook("restart", w)
}

// installDeps runs the dependencies hook for the app
// and returns your output.
func installDeps(a *App, w io.Writer) error {
	a.log("executing hook dependencies")
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

func (a *App) log(message string) error {
	log.Printf(message)
	l := applog{Date: time.Now(), Message: message}
	a.Logs = append(a.Logs, l)
	return db.Session.Apps().Update(bson.M{"name": a.Name}, a)
}
