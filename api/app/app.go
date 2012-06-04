package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/repository"
	"launchpad.net/mgo/bson"
	"path"
)

type App struct {
	Ip        string
	Machine   int
	Name      string
	Framework string
	State     string
	Teams     []auth.Team
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
	u, err := app.unit()
	if err != nil {
		return err
	}
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
	u, err := app.unit()
	if err != nil {
		return err
	}
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
func (a *App) conf() (string, error) {
	u, err := a.unit()
	if err != nil {
		return "", err
	}
	uRepo, err := repository.GetPath()
	if err != nil {
		return "", err
	}
	cPath := path.Join(uRepo, "app.info")
	cmd := fmt.Sprintf("cat %s", cPath)
	output, err := u.Command(cmd)
	return string(output), err
}

/*
* This function is responsible for running user's pre-restart scripts.
* Those scripts can be found at the app.conf file, at the root of user's app repository.
 */
// func (a *App) preRestart() error {
// 	return nil
// }

func (a *App) updateHooks() error {
	u, err := a.unit()
	if err != nil {
		return err
	}
	err = u.ExecuteHook("dependencies")
	if err != nil {
		return err
	}
	err = u.ExecuteHook("reload-gunicorn")
	if err != nil {
		return err
	}
	return nil
}

func (app *App) unit() (unit.Unit, error) {
	return unit.Unit{Name: app.Name, Type: app.Framework, Machine: app.Machine}, nil
}
