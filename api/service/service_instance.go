package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type ServiceInstance struct {
	Name        string   `bson:"_id"`
	ServiceName string   `bson:"service_name"`
	Apps        []string `bson:"apps"`
}

func (si *ServiceInstance) Create() error {
	err := db.Session.ServiceInstances().Insert(si)
	return err
	// s := si.Service()
	// apps := si.AllApps()
	// appUnit := unit.Unit{Name: apps[0].Name}
	// serviceUnit := unit.Unit{Name: s.Name}
	// appUnit.AddRelation(&serviceUnit)
	/* return nil */
}

func (si *ServiceInstance) Delete() error {
	doc := bson.M{"_id": si.Name, "apps": si.Apps}
	err := db.Session.ServiceInstances().Remove(doc)
	return err
	// s := si.Service()
	// a := si.AllApps()
	// appUnit := unit.Unit{Name: a.Name}
	// serviceUnit := unit.Unit{Name: s.Name}
	// appUnit.RemoveRelation(&serviceUnit)
	/* return nil */
}

func (si *ServiceInstance) Service() *Service {
	//s := &Service{"_id": si.ServiceName}
	s := &Service{}
	db.Session.Services().Find(bson.M{"_id": si.ServiceName}).One(&s)
	return s
}

func (si *ServiceInstance) AllApps() []app.App {
	var apps []app.App
	q := bson.M{"name": bson.M{"$in": si.Apps}}
	db.Session.Apps().Find(q).All(&apps)
	return apps
}
