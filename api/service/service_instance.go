package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type ServiceInstance struct {
	Name        string `bson:"_id"`
	ServiceName string `bson:"service_name"`
	Apps        []string
	Teams       []string
	Instance    string
	Host        string
	State       string
	Env         map[string]string
}

func (si *ServiceInstance) Create() error {
	if si.State == "" {
		si.State = "creating"
	}
	return db.Session.ServiceInstances().Insert(si)
}

func (si *ServiceInstance) Delete() error {
	doc := bson.M{"_id": si.Name, "apps": si.Apps}
	return db.Session.ServiceInstances().Remove(doc)
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

func (si *ServiceInstance) RemoveApp(appName string) {
	var i int
	var name string
	for i, name = range si.Apps {
		if name == appName {
			break
		}
	}
	last := len(si.Apps) - 1
	if i != last {
		si.Apps[i] = si.Apps[last]
	}
	si.Apps = si.Apps[:last]
}
