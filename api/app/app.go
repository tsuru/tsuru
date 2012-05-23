package app

import (
	"errors"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"launchpad.net/mgo/bson"
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

func (app *App) stop() error {
	u, err := app.unit()
	if err != nil {
		return err
	}
	log.Printf("stopping %s app", app.Name)
	return u.ExecuteHook("stop")
}

func (app *App) start() error {
	u, err := app.unit()
	if err != nil {
		return err
	}
	log.Printf("starting %s app", app.Name)
	return u.ExecuteHook("start")
}

func (app *App) restart() error {
	err := app.stop()
	if err != nil {
		return err
	}
	return app.start()
}

func (app *App) unit() (unit.Unit, error) {
	return unit.Unit{Name: app.Name, Type: app.Framework}, nil
}
