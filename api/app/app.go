package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"github.com/timeredbull/tsuru/repository"
	"launchpad.net/goyaml"
	"launchpad.net/mgo/bson"
	"path"
	"strings"
)

const confSep = "========"

type App struct {
	Env       map[string]string
	Framework string
	Ip        string
	Machine   int
	Name      string
	State     string
	Teams     []auth.Team
}

type conf struct {
	PreRestart string "pre-restart"
	PosRestart string "pos-restart"
}

func AllApps() ([]App, error) {
	var apps []App
	err := db.Session.Apps().Find(nil).All(&apps)
	return apps, err
}

func (a *App) Get() error {
	return db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
}

func (a *App) Create() error {
	a.State = "Pending"
	err := db.Session.Apps().Insert(a)
	if err != nil {
		return err
	}
	u := a.unit()
	err = u.Create()
	if err != nil {
		return err
	}
	return nil
}

func (a *App) Destroy() error {
	err := db.Session.Apps().Remove(a)
	if err != nil {
		return err
	}
	u := a.unit()
	u.Destroy()
	return nil
}

func (a *App) findTeam(team *auth.Team) int {
	for i, t := range a.Teams {
		if t.Name == team.Name {
			return i
		}
	}
	return -1
}

func (a *App) hasTeam(team *auth.Team) bool {
	return a.findTeam(team) > -1
}

func (a *App) GrantAccess(team *auth.Team) error {
	if a.hasTeam(team) {
		return errors.New("This team has already access to this app")
	}
	a.Teams = append(a.Teams, *team)
	return nil
}

func (a *App) RevokeAccess(team *auth.Team) error {
	index := a.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this app")
	}
	last := len(a.Teams) - 1
	a.Teams[index] = a.Teams[last]
	a.Teams = a.Teams[:last]
	return nil
}

func (a *App) CheckUserAccess(user *auth.User) bool {
	for _, team := range a.Teams {
		if team.ContainsUser(user) {
			return true
		}
	}
	return false
}

func (a *App) SetEnv(name, value string) {
	if a.Env == nil {
		a.Env = make(map[string]string)
	}
	a.Env[name] = value
}

func (a *App) GetEnv(name string) (value string, err error) {
	var ok bool
	if value, ok = a.Env[name]; !ok {
		err = errors.New("Environment variable not declared for this app.")
	}
	return
}

func deployHookAbsPath(p string) (string, error) {
	repoPath, err := config.GetString("git:unit-repo")
	if err != nil {
		return "", nil
	}
	return path.Join(repoPath, p), nil
}

/*
* Returns app.conf located at app's git repository
 */
func (a *App) conf() (conf, error) {
	var c conf
	u := a.unit()
	uRepo, err := repository.GetPath()
	if err != nil {
		log.Printf("Got error while getting repository path: %s", err.Error())
		return c, err
	}
	cPath := path.Join(uRepo, "app.conf")
	cmd := fmt.Sprintf(`echo "%s";cat %s`, confSep, cPath)
	o, err := u.Command(cmd)
	if err != nil {
		log.Printf("Got error while executing command: %s... Skipping hooks execution", err.Error())
		return c, nil
	}
	data := strings.Split(string(o), confSep)[1]
	err = goyaml.Unmarshal([]byte(data), &c)
	if err != nil {
		log.Printf("Got error while parsing yaml: %s", err.Error())
		return c, err
	}
	return c, nil
}

/*
* preRestart is responsible for running user's pre-restart script.
* The path to this script can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) preRestart(c conf) error {
	if !a.hasRestartHooks(c) {
		log.Printf("app.conf file does not exists or is in the right place. Skipping...")
		return nil
	}
	if c.PreRestart == "" {
		log.Printf("pre-restart hook section in app conf does not exists... Skipping...")
		return nil
	}
	u := a.unit()
	p, err := deployHookAbsPath(c.PreRestart)
	if err != nil {
		log.Printf("Error obtaining absolute path to hook: %s. Skipping...", err)
		return nil
	}
	out, err := u.Command("/bin/bash", p)
	log.Printf("Executing pre-restart hook...")
	log.Printf("Output of pre-restart hook:", string(out))
	log.Printf(string(out))
	return err
}

/*
* posRestart is responsible for running user's pos-restart script.
* The path to this script can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) posRestart(c conf) error {
	if !a.hasRestartHooks(c) {
		log.Printf("app.conf file does not exists or is in the right place. Skipping...")
		return nil
	}
	if c.PosRestart == "" {
		log.Printf("pos-restart hook section in app conf does not exists... Skipping...")
		return nil
	}
	u := a.unit()
	p, err := deployHookAbsPath(c.PosRestart)
	if err != nil {
		log.Printf("Error obtaining absolute path to hook: %s. Skipping...", err)
		return nil
	}
	out, err := u.Command("/bin/bash", p)
	log.Printf("Executing pos-restart hook...")
	log.Printf("Output of pos-restart hook: %s", string(out))
	return err
}

func (a *App) hasRestartHooks(c conf) bool {
	return !(c.PreRestart == "" && c.PosRestart == "")
}

func (a *App) updateHooks() error {
	u := a.unit()
	err := u.ExecuteHook("dependencies")
	if err != nil {
		return err
	}
	err = u.ExecuteHook("reload-gunicorn")
	if err != nil {
		return err
	}
	return nil
}

func (a *App) unit() unit.Unit {
	return unit.Unit{Name: a.Name, Type: a.Framework, Machine: a.Machine}
}
