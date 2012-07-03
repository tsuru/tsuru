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
	Instances   []string
	Host        string
}

func (si *ServiceInstance) Create() error {
	err := db.Session.ServiceInstances().Insert(si)
	return err
}

func (si *ServiceInstance) Delete() error {
	doc := bson.M{"_id": si.Name, "apps": si.Apps}
	err := db.Session.ServiceInstances().Remove(doc)
	return err
}

func (si *ServiceInstance) Service() *Service {
	s := &Service{}
	db.Session.Services().Find(bson.M{"_id": si.ServiceName}).One(s)
	return s
}

func (si *ServiceInstance) AllApps() []app.App {
	var apps []app.App
	q := bson.M{"name": bson.M{"$in": si.Apps}}
	db.Session.Apps().Find(q).All(&apps)
	return apps
}
