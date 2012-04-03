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
	apps := make([]App, 100)

	c := Mdb.C("apps")
	iter := c.Find(nil).Iter()
	err := iter.All(&apps)
	if err != nil {
		panic(iter.Err())
	}

	return apps, err
}

func (app *App) Get() error {
	query := make(map[string]interface{})
	query["name"] = app.Name

	c := Mdb.C("apps")
	err := c.Find(query).One(&app)

	if err != nil {
		return err
	}

	return nil
}

func (app *App) Create() error {
	app.State = "Pending"
	app.Id = bson.NewObjectId()

	c := Mdb.C("apps")
	doc := bson.M{"_id": app.Id, "name": app.Name, "framework": app.Framework, "state": app.State, "ip": app.Ip}
	err := c.Insert(doc)
	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	err = u.Create()

	return nil
}

func (app *App) Destroy() error {
	c := Mdb.C("apps")
	doc := bson.M{"name": app.Name}
	err := c.Remove(doc)

	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}
