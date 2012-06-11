package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
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
	Ip        string
	Machine   int
	Name      string
	Framework string
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
		return errors.New("This team has already access to this a")
	}
	a.Teams = append(a.Teams, *team)
	return nil
}

func (a *App) RevokeAccess(team *auth.Team) error {
	index := a.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this a")
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

/*
* Returns app.conf located at app's git repository
 */
func (a *App) conf() (conf, error) {
	var c conf
	u := a.unit()
	uRepo, err := repository.GetPath()
	if err != nil {
		return c, err
	}
	cPath := path.Join(uRepo, "app.info")
	cmd := fmt.Sprintf(`echo "%s";cat %s`, confSep, cPath)
	o, err := u.Command(cmd)
	data := strings.Split(string(o), confSep)[1]
	err = goyaml.Unmarshal([]byte(data), &c)
	if err != nil {
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
		return errors.New("app.conf file does not exists or is in the right place.")
	}
	if c.PreRestart == "" {
		log.Printf("pre-restart hook section in app conf does not exists... Skipping...")
		return nil
	}
	u := a.unit()
	out, err := u.Command("/bin/bash", c.PreRestart)
	log.Printf("Executing pre-restart hook...")
	log.Printf(string(out))
	return err
}

/*
* posRestart is responsible for running user's pos-restart script.
* The path to this script can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) posRestart(c conf) error {
	if !a.hasRestartHooks(c) {
		return errors.New("app.conf file does not exists or is in the right place.")
	}
	if c.PosRestart == "" {
		log.Printf("pos-restart hook section in app conf does not exists... Skipping...")
		return nil
	}
	u := a.unit()
	out, err := u.Command("/bin/bash", c.PosRestart)
	log.Printf("Executing pos-restart hook...")
	log.Printf(string(out))
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
