package app

import (
	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/database"
	"launchpad.net/mgo/bson"
)

type App struct {
	Id        bson.ObjectId "_id"
	Ip        string
	Name      string
	Framework string
	State     string
}

func AllApps() ([]App, error) {
	var apps []App
	c := Mdb.C("apps")
	err := c.Find(nil).All(&apps)
	return apps, err
}

func (app *App) Get() error {
	c := Mdb.C("apps")
	return c.Find(bson.M{"name": app.Name}).One(&app)
}

func (app *App) Create() error {
	app.State = "Pending"
	app.Id = bson.NewObjectId()

	c := Mdb.C("apps")
	err := c.Insert(app)
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
	c := Mdb.C("apps")
	err := c.Remove(app)

	if err != nil {
		return err
	}
	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}
