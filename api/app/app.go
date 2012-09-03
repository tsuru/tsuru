package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"github.com/timeredbull/tsuru/repository"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"
)

const confSep = "========"

type authorizer interface {
	authorize(*App) error
	setCreds(access string, secret string)
}

type App struct {
	Env         map[string]bind.EnvVar
	Framework   string
	JujuEnv     string
	KeystoneEnv keystoneEnv
	Logs        []applog
	Name        string
	State       string
	Units       []Unit
	Teams       []string
	ec2Auth     authorizer
}

type applog struct {
	Date    time.Time
	Message string
}

type conf struct {
	PreRestart string `yaml:"pre-restart"`
	PosRestart string `yaml:"pos-restart"`
}

func (a *App) Get() error {
	return db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
}

// createApp creates a new app.
//
// Creating a new app is a big process that can be divided in some steps (and
// two scenarios):
//
//   Scenario 1: Multi tenancy enabled
//
//       1. Creates keystone credentials for the app
//       2. Write the juju environment to juju's environments file
//       3. Bootstrap juju environment
//       4. Authorizes ssh and http access to the app instance
//       5. Saves the app in the database
//       6. Deploys juju charm
//
//   Scenario 2: Multi tenancy disabled
//
//       1. Sets app juju env to the default juju env (defined in the
//          tsuru.conf file)
//       2. Saves the app in the database
//       3. Deploys juju charm
//
// Multi tenancy should be configured in tsuru's conf file
// (set the "multi-tenant" flag to true or false, as desired).
func createApp(a *App) error {
	var err error
	isMultiTenant, err := config.GetBool("multi-tenant")
	if err != nil {
		return err
	}
	if isMultiTenant {
		a.KeystoneEnv, err = newKeystoneEnv(a.Name)
		if err != nil {
			return err
		}
		err = newEnviron(a)
		if err != nil {
			return err
		}
		if a.JujuEnv == "" {
			a.JujuEnv = a.Name
		}
		cmd := exec.Command("juju", "bootstrap", "-e", a.JujuEnv)
		log.Printf("bootstraping juju environment %s for the app %s", a.JujuEnv, a.Name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("failed to bootstrap juju environment %s:\n%s", a.JujuEnv, out)
			return fmt.Errorf("Failed to bootstrap juju env (%s): %s", err, out)
		}
		authorizer := a.authorizer()
		authorizer.setCreds(a.KeystoneEnv.AccessKey, a.KeystoneEnv.secretKey)
		err = authorizer.authorize(a)
		if err != nil {
			return fmt.Errorf("Failed to create the app, it was not possible to authorize the access to the app: %s", err)
		}
	} else {
		a.JujuEnv, err = config.GetString("juju:default-env")
		if err != nil {
			return err
		}
	}
	a.State = "pending"
	err = db.Session.Apps().Insert(a)
	if err != nil {
		return err
	}
	a.log(fmt.Sprintf("creating app %s", a.Name))
	cmd := exec.Command("juju", "deploy", "-e", a.JujuEnv, "--repository=/home/charms", "local:"+a.Framework, a.Name)
	log.Printf("deploying %s with name %s on environment %s", a.Framework, a.Name, a.JujuEnv)
	out, err := cmd.CombinedOutput()
	a.log(string(out))
	if err != nil {
		a.log(fmt.Sprintf("juju finished with exit status: %s", err.Error()))
		db.Session.Apps().Remove(bson.M{"name": a.Name})
		return errors.New(string(out))
	}
	a.log(fmt.Sprintf("app %s successfully created", a.Name))
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
	multitenant, err := config.GetBool("multi-tenant")
	if err != nil {
		return errors.New("multi-tenant flag not defined in config file. You need to define this flag.")
	}
	if multitenant {
		destroyCmd := exec.Command("juju", "destroy-environment", "-e", a.JujuEnv)
		destroyCmd.Stdin = strings.NewReader("y")
		if out, err := destroyCmd.CombinedOutput(); err != nil {
			msg := fmt.Sprintf("Failed to destroy juju-environment:\n%s", out)
			log.Print(msg)
			return errors.New(string(out))
		}
		if err = destroyKeystoneEnv(&a.KeystoneEnv); err != nil {
			return err
		}
		if err = a.KeystoneEnv.disassociate(); err != nil {
			return err
		}
	} else {
		out, err := a.unit().destroy()
		msg := string(out)
		log.Printf(msg)
		if err != nil {
			return errors.New(msg)
		}
	}
	unbindCh := make(chan error)
	go func() {
		unbindCh <- a.unbind()
	}()
	err = db.Session.Apps().Remove(bson.M{"name": a.Name})
	if err != nil {
		return err
	}
	return <-unbindCh
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
	last := len(a.Teams) - 1
	a.Teams[index] = a.Teams[last]
	a.Teams = a.Teams[:last]
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
	return path.Join(repoPath, p), nil
}

/*
 Returns app.conf located at app's git repository
*/
func (a *App) conf() (conf, error) {
	var c conf
	uRepo, err := repository.GetPath()
	if err != nil {
		a.log(fmt.Sprintf("Got error while getting repository path: %s", err.Error()))
		return c, err
	}
	cPath := path.Join(uRepo, "app.conf")
	cmd := fmt.Sprintf(`echo "%s";cat %s`, confSep, cPath)
	o, err := a.unit().Command(cmd)
	if err != nil {
		a.log(fmt.Sprintf("Got error while executing command: %s... Skipping hooks execution", err.Error()))
		return c, nil
	}
	data := strings.Split(string(o), confSep)[1]
	err = goyaml.Unmarshal([]byte(data), &c)
	if err != nil {
		a.log(fmt.Sprintf("Got error while parsing yaml: %s", err.Error()))
		return c, err
	}
	return c, nil
}

func (a *App) authorizer() authorizer {
	if a.ec2Auth == nil {
		a.ec2Auth = &ec2Authorizer{}
	}
	return a.ec2Auth
}

/*
* preRestart is responsible for running user's pre-restart script.
* The path to this script can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) preRestart(c conf) ([]byte, error) {
	if !a.hasRestartHooks(c) {
		msg := "app.conf file does not exists or is in the right place. Skipping pre-restart hook..."
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	if c.PreRestart == "" {
		msg := "pre-restart hook section in app conf does not exists... Skipping pre-restart hook..."
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	p, err := deployHookAbsPath(c.PreRestart)
	if err != nil {
		msg := fmt.Sprintf("Error obtaining absolute path to hook: %s. Skipping pre-restart hook...", err)
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	a.log("Executing pre-restart hook...")
	out, err := a.unit().Command("/bin/bash", p)
	a.log(fmt.Sprintf("Output of pre-restart hook: %s", string(out)))
	return out, err
}

/*
* posRestart is responsible for running user's pos-restart script.
* The path to this script can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) posRestart(c conf) ([]byte, error) {
	if !a.hasRestartHooks(c) {
		msg := "app.conf file does not exists or is in the right place. Skipping pos-restart hook..."
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	if c.PosRestart == "" {
		msg := "pos-restart hook section in app conf does not exists... Skipping pos-restart hook..."
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	p, err := deployHookAbsPath(c.PosRestart)
	if err != nil {
		msg := fmt.Sprintf("Error obtaining absolute path to hook: %s. Skipping pos-restart-hook...", err)
		a.log(msg)
		return []byte(msg + "\n"), nil
	}
	out, err := a.unit().Command("/bin/bash", p)
	a.log("Executing pos-restart hook...")
	a.log(fmt.Sprintf("Output of pos-restart hook: %s", string(out)))
	return out, err
}

func (a *App) hasRestartHooks(c conf) bool {
	return !(c.PreRestart == "" && c.PosRestart == "")
}

func (a *App) updateHooks() ([]byte, error) {
	u := a.unit()
	a.log("executting hook dependencies")
	out, err := u.executeHook("dependencies")
	a.log(string(out))
	if err != nil {
		return out, err
	}
	a.log("executting hook to restarting")
	restartOut, err := u.executeHook("restart")
	out = append(out, restartOut...)
	if err != nil {
		return out, err
	}
	a.log(string(out))
	return out, nil
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
		units = append(units, bind.Unit(&u))
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
