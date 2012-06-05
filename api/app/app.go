package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/repository"
	"io"
	"launchpad.net/goyaml"
	"launchpad.net/mgo/bson"
	"log"
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

func (app *App) Get() error {
	return db.Session.Apps().Find(bson.M{"name": app.Name}).One(&app)
}

func (app *App) Create() error {
	app.State = "Pending"
	err := db.Session.Apps().Insert(app)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	u := app.unit()
	err = u.Create()
	if err != nil {
		return err
	}
	return nil
}

func (app *App) Destroy() error {
	err := db.Session.Apps().Remove(app)
	if err != nil {
		return err
	}
	u := app.unit()
	u.Destroy()
	return nil
}

func (app *App) findTeam(team *auth.Team) int {
	for i, t := range app.Teams {
		if t.Name == team.Name {
			return i
		}
	}
	return -1
}

func (app *App) hasTeam(team *auth.Team) bool {
	return app.findTeam(team) > -1
}

func (app *App) GrantAccess(team *auth.Team) error {
	if app.hasTeam(team) {
		return errors.New("This team has already access to this app")
	}
	app.Teams = append(app.Teams, *team)
	return nil
}

func (app *App) RevokeAccess(team *auth.Team) error {
	index := app.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this app")
	}
	last := len(app.Teams) - 1
	app.Teams[index] = app.Teams[last]
	app.Teams = app.Teams[:last]
	return nil
}

func (app *App) CheckUserAccess(user *auth.User) bool {
	for _, team := range app.Teams {
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
* preRestart is responsible for running user's pre-restart scripts.
* Those scripts can be found at the app.conf file, at the root of user's app repository.
 */
func (a *App) preRestart(c conf, w io.Writer) error {
	log.SetOutput(w)
	u := a.unit()
	out, err := u.Command("/bin/bash", c.PreRestart)
	log.Printf("Executing pre-restart hook...")
	log.Printf(string(out))
	return err
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

func (app *App) unit() unit.Unit {
	return unit.Unit{Name: app.Name, Type: app.Framework, Machine: app.Machine}
}
