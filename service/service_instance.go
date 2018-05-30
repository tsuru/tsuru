// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var (
	ErrServiceInstanceNotFound   = errors.New("service instance not found")
	ErrInvalidInstanceName       = errors.New("invalid service instance name")
	ErrInstanceNameAlreadyExists = errors.New("instance name already exists.")
	ErrAccessNotAllowed          = errors.New("user does not have access to this service instance")
	ErrTeamMandatory             = errors.New("please specify the team that owns the service instance")
	ErrAppAlreadyBound           = errors.New("app is already bound to this service instance")
	ErrAppNotBound               = errors.New("app is not bound to this service instance")
	ErrUnitNotBound              = errors.New("unit is not bound to this service instance")
	ErrServiceInstanceBound      = errors.New("This service instance is bound to at least one app. Unbind them before removing it")
	instanceNameRegexp           = regexp.MustCompile(`^[A-Za-z][-a-zA-Z0-9_]+$`)
)

type ServiceInstance struct {
	Name        string   `json:"name"`
	Id          int      `json:"id"`
	ServiceName string   `bson:"service_name" json:"service_name"`
	PlanName    string   `bson:"plan_name" json:"plan_name"`
	Apps        []string `json:"apps"`
	BoundUnits  []Unit   `bson:"bound_units" json:"bound_units"`
	Teams       []string `json:"teams"`
	TeamOwner   string   `json:"team_owner"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type Unit struct {
	AppName string `json:"app_name"`
	ID      string `json:"id"`
	IP      string `json:"ip"`
}

func (bu Unit) GetID() string {
	return bu.ID
}

func (bu Unit) GetIp() string {
	return bu.IP
}

// DeleteInstance deletes the service instance from the database.
func DeleteInstance(si *ServiceInstance, evt *event.Event, requestID string) error {
	if len(si.Apps) > 0 {
		return ErrServiceInstanceBound
	}
	endpoint, err := si.Service().getClient("production")
	if err == nil {
		endpoint.Destroy(si, evt, requestID)
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

func (si *ServiceInstance) Info(requestID string) (map[string]string, error) {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return nil, errors.New("endpoint does not exists")
	}
	result, err := endpoint.Info(si, requestID)
	if err != nil {
		return nil, err
	}
	info := map[string]string{}
	for _, d := range result {
		info[d["label"]] = d["value"]
	}
	return info, nil
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

// Update changes informations of the service instance.
func (si *ServiceInstance) Update(service Service, updateData ServiceInstance, evt *event.Event, requestID string) error {
	err := validateServiceInstanceTeamOwner(updateData)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	tags := processTags(updateData.Tags)
	if tags == nil {
		updateData.Tags = si.Tags
	} else {
		updateData.Tags = tags
	}
	actions := []*action.Action{&updateServiceInstance, &notifyUpdateServiceInstance}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(service, *si, updateData, evt, requestID)
}

func (si *ServiceInstance) updateData(update bson.M) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName}, update)
}

// BindApp makes the bind between the service instance and an app.
func (si *ServiceInstance) BindApp(app bind.App, shouldRestart bool, writer io.Writer, evt *event.Event, requestID string) error {
	args := bindPipelineArgs{
		serviceInstance: si,
		app:             app,
		writer:          writer,
		shouldRestart:   shouldRestart,
		event:           evt,
		requestID:       requestID,
	}
	actions := []*action.Action{
		bindAppDBAction,
		bindAppEndpointAction,
		setBoundEnvsAction,
		bindUnitsAction,
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
	updateOp := bson.M{
		"$addToSet": bson.M{
			"bound_units": bson.D([]bson.DocElem{
				{Name: "appname", Value: app.GetName()},
				{Name: "id", Value: unit.GetID()},
				{Name: "ip", Value: unit.GetIp()},
			}),
		},
	}
	err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "bound_units.id": bson.M{"$ne": unit.GetID()}}, updateOp)
	conn.Close()
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return err
	}
	err = endpoint.BindUnit(si, app, unit)
	if err != nil {
		updateOp = bson.M{
			"$pull": bson.M{
				"bound_units": bson.D([]bson.DocElem{
					{Name: "appname", Value: app.GetName()},
					{Name: "id", Value: unit.GetID()},
					{Name: "ip", Value: unit.GetIp()},
				}),
			},
		}
		rollbackErr := si.updateData(updateOp)
		if rollbackErr != nil {
			log.Errorf("[bind unit] could not remove stil unbound unit from db after failure: %s", rollbackErr)
		}
		return err
	}
	return nil
}

type UnbindAppArgs struct {
	App         bind.App
	Restart     bool
	ForceRemove bool
	Event       *event.Event
	RequestID   string
}

// UnbindApp makes the unbind between the service instance and an app.
func (si *ServiceInstance) UnbindApp(unbindArgs UnbindAppArgs) error {
	if si.FindApp(unbindArgs.App.GetName()) == -1 {
		return ErrAppNotBound
	}
	args := bindPipelineArgs{
		serviceInstance: si,
		app:             unbindArgs.App,
		writer:          unbindArgs.Event,
		shouldRestart:   unbindArgs.Restart,
		event:           unbindArgs.Event,
		requestID:       unbindArgs.RequestID,
		forceRemove:     unbindArgs.ForceRemove,
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
	updateOp := bson.M{
		"$pull": bson.M{
			"bound_units": bson.D([]bson.DocElem{
				{Name: "appname", Value: app.GetName()},
				{Name: "id", Value: unit.GetID()},
				{Name: "ip", Value: unit.GetIp()},
			}),
		},
	}
	err = conn.ServiceInstances().Update(bson.M{"name": si.Name, "service_name": si.ServiceName, "bound_units.id": unit.GetID()}, updateOp)
	if err != nil {
		if err == mgo.ErrNotFound {
			return ErrUnitNotBound
		}
		return err
	}
	err = endpoint.UnbindUnit(si, app, unit)
	if err != nil {
		updateOp = bson.M{
			"$addToSet": bson.M{
				"bound_units": bson.D([]bson.DocElem{
					{Name: "appname", Value: app.GetName()},
					{Name: "id", Value: unit.GetID()},
					{Name: "ip", Value: unit.GetIp()},
				}),
			},
		}
		rollbackErr := si.updateData(updateOp)
		if rollbackErr != nil {
			log.Errorf("[unbind unit] could not add bound unit back to db after failure: %s", rollbackErr)
		}
		return err
	}
	return nil
}

// Status returns the service instance status.
func (si *ServiceInstance) Status(requestID string) (string, error) {
	endpoint, err := si.Service().getClient("production")
	if err != nil {
		return "", err
	}
	return endpoint.Status(si, requestID)
}

func (si *ServiceInstance) Grant(teamName string) error {
	team, err := servicemanager.Team.FindByName(teamName)
	if err != nil {
		return err
	}
	return si.updateData(bson.M{"$push": bson.M{"teams": team.Name}})
}

func (si *ServiceInstance) Revoke(teamName string) error {
	team, err := servicemanager.Team.FindByName(teamName)
	if err != nil {
		return err
	}
	return si.updateData(bson.M{"$pull": bson.M{"teams": team.Name}})
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

func validateServiceInstance(si ServiceInstance, s *Service) error {
	err := validateServiceInstanceName(s.Name, si.Name)
	if err != nil {
		return err
	}
	return validateServiceInstanceTeamOwner(si)
}

func validateServiceInstanceName(service, instance string) error {
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
	if err != nil {
		return err
	}
	if length > 0 {
		return ErrInstanceNameAlreadyExists
	}
	return nil
}

func validateServiceInstanceTeamOwner(si ServiceInstance) error {
	if si.TeamOwner == "" {
		return ErrTeamMandatory
	}
	_, err := servicemanager.Team.FindByName(si.TeamOwner)
	if err == authTypes.ErrTeamNotFound {
		return fmt.Errorf("Team owner doesn't exist")
	}
	return err
}

func CreateServiceInstance(instance ServiceInstance, service *Service, evt *event.Event, requestID string) error {
	err := validateServiceInstance(instance, service)
	if err != nil {
		return err
	}
	instance.ServiceName = service.Name
	instance.Teams = []string{instance.TeamOwner}
	instance.Tags = processTags(instance.Tags)
	actions := []*action.Action{&notifyCreateServiceInstance, &createServiceInstance}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(*service, instance, evt, requestID)
}

func GetServiceInstancesByServices(services []Service) ([]ServiceInstance, error) {
	var instances []ServiceInstance
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	query := genericServiceInstancesFilter(services, []string{})
	f := bson.M{"name": 1, "service_name": 1, "tags": 1}
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

func GetServiceInstancesBoundToApp(appName string) ([]ServiceInstance, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instances []ServiceInstance
	q := bson.M{"apps": bson.M{"$in": []string{appName}}}
	err = conn.ServiceInstances().Find(q).All(&instances)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func processTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	processedTags := []string{}
	usedTags := make(map[string]bool)
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 0 && !usedTags[tag] {
			processedTags = append(processedTags, tag)
			usedTags[tag] = true
		}
	}
	return processedTags
}

func RenameServiceInstanceTeam(oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	bulk := conn.ServiceInstances().Bulk()
	bulk.UpdateAll(bson.M{"teamowner": oldName}, bson.M{"$set": bson.M{"teamowner": newName}})
	bulk.UpdateAll(bson.M{"teams": oldName}, bson.M{"$push": bson.M{"teams": newName}})
	bulk.UpdateAll(bson.M{"teams": oldName}, bson.M{"$pull": bson.M{"teams": oldName}})
	_, err = bulk.Run()
	return err
}

// ProxyInstance is a proxy between tsuru and the service instance.
// This method allow customized service instance methods.
func ProxyInstance(instance *ServiceInstance, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	service := instance.Service()
	endpoint, err := service.getClient("production")
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("/resources/%s/", instance.GetIdentifier())
	path = strings.Trim(strings.TrimPrefix(path+"/", prefix), "/")
	for _, reserved := range reservedProxyPaths {
		if path == reserved && r.Method != "GET" {
			return &tsuruErrors.ValidationError{
				Message: fmt.Sprintf("proxy request %s %q is forbidden", r.Method, path),
			}
		}
	}
	return endpoint.Proxy(fmt.Sprintf("%s%s", prefix, path), evt, requestID, w, r)
}
