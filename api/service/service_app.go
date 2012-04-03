package service

import (
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	"launchpad.net/mgo/bson"
	. "github.com/timeredbull/tsuru/database"
)

type ServiceApp struct {
	Id        bson.ObjectId "_id"
	ServiceId bson.ObjectId "service_id"
	AppId     bson.ObjectId "app_id"
}

func (sa *ServiceApp) Create() error {
	c := Mdb.C("service_apps")
	sa.Id = bson.NewObjectId()

	err := c.Insert(sa)

	if err != nil {
		panic(err)
	}

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.AddRelation(&serviceUnit)

	return err
}

func (sa *ServiceApp) Delete() error {
	c := Mdb.C("service_apps")
	doc := bson.M{"service_id": sa.ServiceId, "app_id": sa.AppId}
	err := c.Remove(doc)
	if err != nil {
		panic(err)
	}

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.RemoveRelation(&serviceUnit)

	return nil
}

func (sa *ServiceApp) Service() (s *Service) {
	s = &Service{Id: sa.ServiceId}
	s.Get()

	return
}

func (sa *ServiceApp) App() (a *App) {
	query := make(map[string]interface{})
	query["_id"] = sa.AppId

	c := Mdb.C("apps")
	err := c.Find(query).One(&a)
	if err != nil {
		panic(err)
	}

	return
}
