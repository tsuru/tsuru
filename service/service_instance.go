// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	stderrors "errors"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"net/http"
)

type ServiceInstance struct {
	Name        string
	ServiceName string `bson:"service_name"`
	Apps        []string
	Teams       []string
}

func (si *ServiceInstance) Create() error {
	return db.Session.ServiceInstances().Insert(si)
}

func (si *ServiceInstance) Delete() error {
	doc := bson.M{"name": si.Name}
	return db.Session.ServiceInstances().Remove(doc)
}

func (si *ServiceInstance) Service() *Service {
	s := &Service{}
	db.Session.Services().Find(bson.M{"_id": si.ServiceName}).One(s)
	return s
}

func (si *ServiceInstance) AddApp(appName string) error {
	index := si.FindApp(appName)
	if index > -1 {
		return stderrors.New("This instance already has this app.")
	}
	si.Apps = append(si.Apps, appName)
	return nil
}

func (si *ServiceInstance) FindApp(appName string) int {
	index := -1
	for i, name := range si.Apps {
		if name == appName {
			index = i
			break
		}
	}
	return index
}

func (si *ServiceInstance) RemoveApp(appName string) error {
	index := si.FindApp(appName)
	if index < 0 {
		return stderrors.New("This app is not binded to this service instance.")
	}
	copy(si.Apps[index:], si.Apps[index+1:])
	si.Apps = si.Apps[:len(si.Apps)-1]
	return nil
}

func (si *ServiceInstance) update() error {
	return db.Session.ServiceInstances().Update(bson.M{"name": si.Name}, si)
}

func (si *ServiceInstance) Bind(app bind.App) error {
	err := si.AddApp(app.GetName())
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: "This app is already binded to this service instance."}
	}
	var envVars []bind.EnvVar
	var setEnv = func(env map[string]string) {
		for k, v := range env {
			envVars = append(envVars, bind.EnvVar{
				Name:         k,
				Value:        v,
				Public:       false,
				InstanceName: si.Name,
			})
		}
	}
	cli := si.Service().ProductionEndpoint()
	if len(app.GetUnits()) == 0 {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: "This app does not have an IP yet."}
	}
	env, err := cli.Bind(si, app)
	if err != nil {
		return err
	}
	setEnv(env)
	err = si.update()
	if err != nil {
		cli.Unbind(si, app)
		return err
	}
	return app.SetEnvs(envVars, false)
}

func (si *ServiceInstance) Unbind(app bind.App) error {
	err := si.RemoveApp(app.GetName())
	if err != nil {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: "This app is not binded to this service instance."}
	}
	err = si.update()
	if err != nil {
		return err
	}
	go func() {
		si.Service().ProductionEndpoint().Unbind(si, app)
	}()
	var envVars []string
	for k := range app.InstanceEnv(si.Name) {
		envVars = append(envVars, k)
	}
	return app.UnsetEnvs(envVars, false)
}

func genericServiceInstancesFilter(services interface{}, teams []string) (q, f bson.M) {
	f = bson.M{"name": 1, "service_name": 1, "apps": 1}
	q = bson.M{}
	if len(teams) != 0 {
		q["teams"] = bson.M{"$in": teams}
	}
	if v, ok := services.([]Service); ok {
		names := GetServicesNames(v)
		q["service_name"] = bson.M{"$in": names}
	}
	if v, ok := services.(Service); ok {
		q["service_name"] = v.Name
	}
	return
}

func GetServiceInstancesByServices(services []Service) (sInstances []ServiceInstance, err error) {
	q, _ := genericServiceInstancesFilter(services, []string{})
	err = db.Session.ServiceInstances().Find(q).Select(bson.M{"name": 1, "service_name": 1}).All(&sInstances)
	return
}

func GetServiceInstancesByServicesAndTeams(services []Service, u *auth.User) (sInstances []ServiceInstance, err error) {
	teams, err := u.Teams()
	if err != nil {
		return
	}
	if len(teams) == 0 {
		return
	}
	q, f := genericServiceInstancesFilter(services, auth.GetTeamsNames(teams))
	err = db.Session.ServiceInstances().Find(q).Select(f).All(&sInstances)
	return
}
