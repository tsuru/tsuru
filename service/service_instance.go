// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrServiceInstanceNotFound   = errors.New("service instance not found")
	ErrInvalidInstanceName       = errors.New("invalid service instance name")
	ErrInstanceNameAlreadyExists = errors.New("instance name already exists.")
	ErrAccessNotAllowed          = errors.New("user does not have access to this service instance")
	ErrTeamMandatory             = errors.New("please specify the team that owns the service instance")
	ErrAppAlreadyBound           = errors.New("app is already bound to this service instance")
	ErrAppNotBound               = errors.New("app is not bound to this service instance")
	ErrUnitAlreadyBound          = errors.New("unit is already bound to this service instance")
	ErrUnitNotBound              = errors.New("unit is not bound to this service instance")
	ErrServiceInstanceBound      = errors.New("This service instance is bound to at least one app. Unbind them before removing it")
	instanceNameRegexp           = regexp.MustCompile(`^[A-Za-z][-a-zA-Z0-9_]+$`)
)

type ServiceInstance struct {
	Name        string
	Id          int
	ServiceName string `bson:"service_name"`
	PlanName    string `bson:"plan_name"`
	Apps        []string
	Units       []string
	Teams       []string
	TeamOwner   string
}

// DeleteInstance deletes the service instance from the database.
func DeleteInstance(si *ServiceInstance) error {
	if len(si.Apps) > 0 {
		return ErrServiceInstanceBound
	}
	endpoint, err := si.Service().getClient("production")
	if err == nil {
		endpoint.Destroy(si)
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.ServiceInstances().Remove(bson.M{"name": si.Name, "service_name": si.ServiceName})
}

func (si *ServiceInstance) GetIdentifier() string {
	if si.Id != 0 {
		return strconv.Itoa(si.Id)
	}
	return si.Name
}

// MarshalJSON marshals the ServiceName in json format.
func (si *ServiceInstance) MarshalJSON() ([]byte, error) {
	info, err := si.Info()
	if err != nil {
		info = nil
	}
	data := map[string]interface{}{
		"Id":          si.Id,
		"Name":        si.Name,
		"Teams":       si.Teams,
		"PlanName":    si.PlanName,
		"Apps":        si.Apps,
		"ServiceName": si.ServiceName,
		"Info":        info,
	}
	return json.Marshal(&data)
}

func (si *ServiceInstance) Info() (map[string]string, error) {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return nil, errors.New("endpoint does not exists")
	}
	result, err := endpoint.Info(si)
	if err != nil {
		return nil, err
	}
	info := map[string]string{}
	for _, d := range result {
		info[d["label"]] = d["value"]
	}
	return info, nil
}

func (si *ServiceInstance) Create() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.ServiceInstances().Insert(si)
}

func (si *ServiceInstance) Service() *Service {
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
		return nil
	}
	defer conn.Close()
	var s Service
	conn.Services().Find(bson.M{"_id": si.ServiceName}).One(&s)
	return &s
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

func (si *ServiceInstance) update(update bson.M) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName}, update)
}

// BindApp makes the bind between the service instance and an app.
func (si *ServiceInstance) BindApp(app bind.App, shouldRestart bool, writer io.Writer) error {
	args := bindPipelineArgs{
		serviceInstance: si,
		app:             app,
		writer:          writer,
		shouldRestart:   shouldRestart,
	}
	actions := []*action.Action{
		&bindAppDBAction,
		&bindAppEndpointAction,
		&setBoundEnvsAction,
		&bindUnitsAction,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(&args)
}

// BindUnit makes the bind between the binder and an unit.
func (si *ServiceInstance) BindUnit(app bind.App, unit bind.Unit) error {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	updateOp := bson.M{"$addToSet": bson.M{"units": unit.GetID()}}
	err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "units": bson.M{"$ne": unit.GetID()}}, updateOp)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrUnitAlreadyBound
		}
		return err
	}
	err = endpoint.BindUnit(si, app, unit)
	if err != nil {
		rollbackErr := si.update(bson.M{"$pull": bson.M{"units": unit.GetID()}})
		if rollbackErr != nil {
			log.Errorf("[bind unit] could not remove stil unbound unit from db after failure: %s", rollbackErr)
		}
		return err
	}
	return nil
}

// UnbindApp makes the unbind between the service instance and an app.
func (si *ServiceInstance) UnbindApp(app bind.App, shouldRestart bool, writer io.Writer) error {
	if si.FindApp(app.GetName()) == -1 {
		return ErrAppNotBound
	}
	args := bindPipelineArgs{
		serviceInstance: si,
		app:             app,
		writer:          writer,
		shouldRestart:   shouldRestart,
	}
	actions := []*action.Action{
		&unbindUnits,
		&unbindAppDB,
		&unbindAppEndpoint,
		&removeBoundEnvs,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(&args)
}

// UnbindUnit makes the unbind between the service instance and an unit.
func (si *ServiceInstance) UnbindUnit(app bind.App, unit bind.Unit) error {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	updateOp := bson.M{"$pull": bson.M{"units": unit.GetID()}}
	err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "units": unit.GetID()}, updateOp)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrUnitNotBound
		}
		return err
	}
	err = endpoint.UnbindUnit(si, app, unit)
	if err != nil {
		rollbackErr := si.update(bson.M{"$addToSet": bson.M{"units": unit.GetID()}})
		if rollbackErr != nil {
			log.Errorf("[unbind unit] could not add bound unit back to db after failure: %s", rollbackErr)
		}
		return err
	}
	return nil
}

// Status returns the service instance status.
func (si *ServiceInstance) Status() (string, error) {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return "", err
	}
	return endpoint.Status(si)
}

func (si *ServiceInstance) Grant(teamName string) error {
	team, err := auth.GetTeam(teamName)
	if err != nil {
		return err
	}
	return si.update(bson.M{"$push": bson.M{"teams": team.Name}})
}

func (si *ServiceInstance) Revoke(teamName string) error {
	team, err := auth.GetTeam(teamName)
	if err != nil {
		return err
	}
	return si.update(bson.M{"$pull": bson.M{"teams": team.Name}})
}

func genericServiceInstancesFilter(services interface{}, teams []string) bson.M {
	query := bson.M{}
	if len(teams) != 0 {
		query["teams"] = bson.M{"$in": teams}
	}
	if v, ok := services.([]Service); ok {
		names := GetServicesNames(v)
		query["service_name"] = bson.M{"$in": names}
	}
	if v, ok := services.(Service); ok {
		query["service_name"] = v.Name
	}
	return query
}

func validateServiceInstanceName(service string, instance string) error {
	if !instanceNameRegexp.MatchString(instance) {
		return ErrInvalidInstanceName
	}
	conn, err := db.Conn()
	if err != nil {
		return nil
	}
	defer conn.Close()
	query := bson.M{"name": instance, "service_name": service}
	length, err := conn.ServiceInstances().Find(query).Count()
	if length > 0 {
		return ErrInstanceNameAlreadyExists
	}
	return nil
}

func CreateServiceInstance(instance ServiceInstance, service *Service, user *auth.User) error {
	err := validateServiceInstanceName(service.Name, instance.Name)
	if err != nil {
		return err
	}
	instance.ServiceName = service.Name
	if instance.TeamOwner == "" {
		return ErrTeamMandatory
	}
	instance.Teams = []string{instance.TeamOwner}
	actions := []*action.Action{&createServiceInstance, &insertServiceInstance}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(*service, instance, user.Email)
}

func GetServiceInstancesByServices(services []Service) ([]ServiceInstance, error) {
	var instances []ServiceInstance
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	query := genericServiceInstancesFilter(services, []string{})
	f := bson.M{"name": 1, "service_name": 1}
	err = conn.ServiceInstances().Find(query).Select(f).All(&instances)
	return instances, err
}

func GetServicesInstancesByTeamsAndNames(teams []string, names []string, appName, serviceName string) ([]ServiceInstance, error) {
	filter := bson.M{}
	if teams != nil || names != nil {
		filter = bson.M{
			"$or": []bson.M{
				{"teams": bson.M{"$in": teams}},
				{"name": bson.M{"$in": names}},
			},
		}
	}
	if appName != "" {
		filter["apps"] = appName
	}
	if serviceName != "" {
		filter["service_name"] = serviceName
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instances []ServiceInstance
	err = conn.ServiceInstances().Find(filter).All(&instances)
	return instances, err
}

func GetServiceInstance(serviceName string, instanceName string) (*ServiceInstance, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instance ServiceInstance
	err = conn.ServiceInstances().Find(bson.M{"name": instanceName, "service_name": serviceName}).One(&instance)
	if err != nil {
		return nil, ErrServiceInstanceNotFound
	}
	return &instance, nil
}
