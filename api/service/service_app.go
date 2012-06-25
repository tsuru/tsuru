package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type ServiceApp struct {
	Id          bson.ObjectId `bson:"_id"`
	ServiceName string        `bson:"service_name"`
	AppName     string        `bson:"app_name"`
}

func (sa *ServiceApp) Create() error {
	sa.Id = bson.NewObjectId()
	err := db.Session.ServiceApps().Insert(sa)
	if err != nil {
		return err
	}

	s := sa.Service()
	a := sa.App()
	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.AddRelation(&serviceUnit)
	return nil
}

func (sa *ServiceApp) Delete() error {
	doc := bson.M{"service_name": sa.ServiceName, "app_name": sa.AppName}
	err := db.Session.ServiceApps().Remove(doc)
	if err != nil {
		return err
	}
	s := sa.Service()
	a := sa.App()
	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.RemoveRelation(&serviceUnit)
	return nil
}

func (sa *ServiceApp) Service() *Service {
	s := &Service{Name: sa.ServiceName}
	s.Get()
	return s
}

func (sa *ServiceApp) App() *app.App {
	var a *app.App
	db.Session.Apps().Find(bson.M{"name": sa.AppName}).One(&a)
	return a
}
