package app

import (
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
	"os"
	"path"
)

const gitServer = "tsuru.plataformas.glb.com"

type App struct {
	Id        bson.ObjectId "_id"
	Ip        string
	Name      string
	Framework string
	State     string
}

type Repository struct {
	Name   string
	Server string
}

func NewRepository(name string) (r *Repository, err error) {
	r = &Repository{
		Name: name,
		Server: gitServer,
	}

	home := os.Getenv("HOME")
	repoPath := path.Join(home, "../git", name)
	err = os.Mkdir(repoPath, 0644)

	return
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
	app.Id = bson.NewObjectId()

	err := db.Session.Apps().Insert(app)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
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
	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}
