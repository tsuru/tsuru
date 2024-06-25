// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

var (
	ErrServiceInstanceNotFound                  = errors.New("service instance not found")
	ErrInvalidInstanceName                      = errors.New("invalid service instance name")
	ErrInstanceNameAlreadyExists                = errors.New("instance name already exists.")
	ErrAccessNotAllowed                         = errors.New("user does not have access to this service instance")
	ErrTeamMandatory                            = errors.New("please specify the team that owns the service instance")
	ErrAppAlreadyBound                          = errors.New("app is already bound to this service instance")
	ErrJobAlreadyBound                          = errors.New("job is already bound to this service instance")
	ErrAppNotBound                              = errors.New("app is not bound to this service instance")
	ErrJobNotBound                              = errors.New("job is not bound to this service instance")
	ErrUnitNotBound                             = errors.New("unit is not bound to this service instance")
	ErrServiceInstanceBound                     = errors.New("This service instance is bound to at least one app. Unbind them before removing it")
	ErrMultiClusterServiceRequiresPool          = errors.New("multi-cluster service instance requires a pool")
	ErrMultiClusterViolatingConstraint          = errors.New("multi-cluster service instance is not allowed in this pool")
	ErrMultiClusterPoolDoesNotMatch             = errors.New("pools between app and multi-cluster service instance does not match")
	ErrRegularServiceInstanceCannotBelongToPool = errors.New("regular (non-multi-cluster) service instance cannot belong to a pool")
	ErrRevokeInstanceTeamOwnerAccess            = errors.New("cannot revoke the instance's team owner access")
	instanceNameRegexp                          = regexp.MustCompile(`^[A-Za-z][-a-zA-Z0-9_]+$`)
)

type ServiceInstance struct {
	Name        string                 `json:"name"`
	Id          int                    `json:"id"`
	ServiceName string                 `bson:"service_name" json:"service_name"`
	PlanName    string                 `bson:"plan_name" json:"plan_name"`
	Apps        []string               `json:"apps"`
	Jobs        []string               `json:"jobs"`
	Teams       []string               `json:"teams"`
	TeamOwner   string                 `json:"team_owner"`
	Description string                 `json:"description"`
	Tags        []string               `json:"tags"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	// Pool is the pool name which the Service Instance should run into.
	// This field is mandatory iff the parent Service is running in
	// multi-cluster mode (see Service.IsMultiCluster field)
	//
	// NOTE: after the service instance is created, this field turns immutable.
	Pool string `json:"pool,omitempty"`

	// BrokerData stores data used by Instances provisioned by Brokers
	BrokerData *BrokerInstanceData `json:"broker_data,omitempty" bson:"broker_data"`

	// ForceRemove indicates whether service instance should be removed even the
	// related call to service API fails.
	ForceRemove bool `bson:"-" json:"-"`
}

type BrokerInstanceData struct {
	// UUID is a v4 UUID generated for this Instance on creation
	UUID             string
	ServiceID        string
	PlanID           string
	OrgID            string
	SpaceID          string
	LastOperationKey string

	Binds map[string]BrokerInstanceBind
}

type BrokerInstanceBind struct {
	// UUID is a v4 UUID generated when binding
	UUID         string
	OperationKey string
	Parameters   map[string]interface{}
}

// DeleteInstance deletes the service instance from the database.
func DeleteInstance(ctx context.Context, si *ServiceInstance, evt *event.Event, requestID string) error {
	if len(si.Apps) > 0 {
		return ErrServiceInstanceBound
	}
	s, err := Get(ctx, si.ServiceName)
	if err != nil {
		return err
	}
	endpoint, err := s.getClientForPool(ctx, si.Pool)
	if err != nil {
		return err
	}
	err = endpoint.Destroy(ctx, si, evt, requestID)
	if err != nil {
		if !si.ForceRemove {
			return err
		}
		fmt.Fprintf(evt, "could not delete the service instance on service api: %v. ignoring this error due to force removal...\n", err)
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

type ServiceInstanceWithInfo struct {
	Id          int
	Name        string
	Pool        string
	Teams       []string
	PlanName    string
	Apps        []string
	Jobs        []string
	ServiceName string
	Info        map[string]string
	TeamOwner   string
}

// ToInfo returns the service instance as a struct compatible with the return
// of the service info api call.
func (si *ServiceInstance) ToInfo(ctx context.Context) (ServiceInstanceWithInfo, error) {
	info, err := si.Info(ctx, "")
	if err != nil {
		info = nil
	}
	return ServiceInstanceWithInfo{
		Id:          si.Id,
		Name:        si.Name,
		Pool:        si.Pool,
		Teams:       si.Teams,
		PlanName:    si.PlanName,
		Apps:        si.Apps,
		Jobs:        si.Jobs,
		ServiceName: si.ServiceName,
		Info:        info,
		TeamOwner:   si.TeamOwner,
	}, nil
}

func (si *ServiceInstance) Info(ctx context.Context, requestID string) (map[string]string, error) {
	s, err := Get(ctx, si.ServiceName)
	if err != nil {
		return nil, err
	}
	endpoint, err := s.getClientForPool(ctx, si.Pool)
	if err != nil {
		return nil, err
	}
	result, err := endpoint.Info(ctx, si, requestID)
	if err != nil {
		return nil, err
	}
	info := map[string]string{}
	for _, d := range result {
		info[d["label"]] = d["value"]
	}
	return info, nil
}

func (si *ServiceInstance) FindApp(appName string) int {
	for i, name := range si.Apps {
		if name == appName {
			return i
		}
	}
	return -1
}

func (si *ServiceInstance) FindJob(jobName string) int {
	for i, name := range si.Jobs {
		if name == jobName {
			return i
		}
	}
	return -1
}

// Update changes informations of the service instance.
func (si *ServiceInstance) Update(ctx context.Context, service Service, updateData ServiceInstance, evt *event.Event, requestID string) error {
	err := validateServiceInstanceTeamOwner(ctx, updateData)
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
	return pipeline.Execute(ctx, service, *si, updateData, evt, requestID)
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
func (si *ServiceInstance) BindApp(ctx context.Context, app bind.App, params BindAppParameters, shouldRestart bool, writer io.Writer, evt *event.Event, requestID string) error {
	args := bindAppPipelineArgs{
		serviceInstance: si,
		app:             app,
		writer:          writer,
		shouldRestart:   shouldRestart,
		params:          params,
		event:           evt,
		requestID:       requestID,
	}
	actions := []*action.Action{
		bindAppDBAction,
		bindAppEndpointAction,
		setBoundEnvsAction,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, &args)
}

// BindJob makes the bind between the service instance and a job.
func (si *ServiceInstance) BindJob(ctx context.Context, job *jobTypes.Job, writer io.Writer, evt *event.Event, requestID string) error {
	args := bindJobPipelineArgs{
		serviceInstance: si,
		job:             job,
		writer:          writer,
		event:           evt,
		requestID:       requestID,
	}
	actions := []*action.Action{
		bindJobDBAction,
		bindJobEndpointAction,
		setJobBoundEnvsAction,
		reloadJobProvisioner,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, &args)
}

type UnbindJobArgs struct {
	Job         *jobTypes.Job
	ForceRemove bool
	Event       *event.Event
	RequestID   string
}

// UnbindJob makes the unbind between the service instance and a job.
func (si *ServiceInstance) UnbindJob(ctx context.Context, unbindArgs UnbindJobArgs) error {
	if si.FindJob(unbindArgs.Job.Name) == -1 {
		return ErrJobNotBound
	}
	args := bindJobPipelineArgs{
		serviceInstance: si,
		job:             unbindArgs.Job,
		writer:          unbindArgs.Event,
		event:           unbindArgs.Event,
		requestID:       unbindArgs.RequestID,
		forceRemove:     unbindArgs.ForceRemove,
	}
	actions := []*action.Action{
		&unbindJobDB,
		&unbindJobEndpoint,
		&removeJobBoundEnvs,
		reloadJobProvisioner,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, &args)
}

type UnbindAppArgs struct {
	App         bind.App
	Restart     bool
	ForceRemove bool
	Event       *event.Event
	RequestID   string
}

// UnbindApp makes the unbind between the service instance and an app.
func (si *ServiceInstance) UnbindApp(ctx context.Context, unbindArgs UnbindAppArgs) error {
	if si.FindApp(unbindArgs.App.GetName()) == -1 {
		return ErrAppNotBound
	}
	args := bindAppPipelineArgs{
		serviceInstance: si,
		app:             unbindArgs.App,
		writer:          unbindArgs.Event,
		shouldRestart:   unbindArgs.Restart,
		event:           unbindArgs.Event,
		requestID:       unbindArgs.RequestID,
		forceRemove:     unbindArgs.ForceRemove,
	}
	actions := []*action.Action{
		&unbindAppDB,
		&unbindAppEndpoint,
		&removeBoundEnvs,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, &args)
}

// Status returns the service instance status.
func (si *ServiceInstance) Status(ctx context.Context, requestID string) (string, error) {
	s, err := Get(ctx, si.ServiceName)
	if err != nil {
		return "", err
	}
	endpoint, err := s.getClientForPool(ctx, si.Pool)
	if err != nil {
		return "", err
	}
	return endpoint.Status(ctx, si, requestID)
}

func (si *ServiceInstance) Grant(ctx context.Context, teamName string) error {
	team, err := servicemanager.Team.FindByName(ctx, teamName)
	if err != nil {
		return err
	}
	return si.updateData(bson.M{"$push": bson.M{"teams": team.Name}})
}

func (si *ServiceInstance) Revoke(ctx context.Context, teamName string) error {
	if teamName == si.TeamOwner {
		return ErrRevokeInstanceTeamOwnerAccess
	}
	team, err := servicemanager.Team.FindByName(ctx, teamName)
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
		names := getServicesNames(v)
		query["service_name"] = bson.M{"$in": names}
	}
	if v, ok := services.(Service); ok {
		query["service_name"] = v.Name
	}
	return query
}

func validateServiceInstance(ctx context.Context, si ServiceInstance, s *Service) error {
	err := validateServiceInstanceName(s.Name, si.Name)
	if err != nil {
		return err
	}
	err = validateServiceInstanceTeamOwner(ctx, si)
	if err != nil {
		return err
	}
	return validateMultiCluster(ctx, s, si)
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

func validateServiceInstanceTeamOwner(ctx context.Context, si ServiceInstance) error {
	if si.TeamOwner == "" {
		return ErrTeamMandatory
	}
	_, err := servicemanager.Team.FindByName(ctx, si.TeamOwner)
	if err == authTypes.ErrTeamNotFound {
		return fmt.Errorf("Team owner doesn't exist")
	}
	return err
}

func CreateServiceInstance(ctx context.Context, instance ServiceInstance, service *Service, evt *event.Event, requestID string) error {
	err := validateServiceInstance(ctx, instance, service)
	if err != nil {
		return err
	}
	instance.ServiceName = service.Name
	instance.Teams = []string{instance.TeamOwner}
	instance.Tags = processTags(instance.Tags)
	actions := []*action.Action{&notifyCreateServiceInstance, &createServiceInstance}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(ctx, *service, &instance, evt, requestID)
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

func GetServiceInstance(ctx context.Context, serviceName string, instanceName string) (*ServiceInstance, error) {
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

func GetServiceInstancesBoundToJob(jobName string) ([]ServiceInstance, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instances []ServiceInstance
	q := bson.M{"jobs": bson.M{"$in": []string{jobName}}}
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

func RenameServiceInstanceTeam(ctx context.Context, oldName, newName string) error {
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
func ProxyInstance(ctx context.Context, instance *ServiceInstance, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	service, err := Get(ctx, instance.ServiceName)
	if err != nil {
		return err
	}
	endpoint, err := service.getClientForPool(ctx, instance.Pool)
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

	return endpoint.Proxy(ctx, &ProxyOpts{
		Instance:  instance,
		Path:      fmt.Sprintf("%s%s", prefix, path),
		Event:     evt,
		RequestID: requestID,
		Writer:    w,
		Request:   r,
	})
}

func validateMultiCluster(ctx context.Context, s *Service, si ServiceInstance) error {
	if !s.IsMultiCluster {
		if si.Pool != "" {
			return ErrRegularServiceInstanceCannotBelongToPool
		}
		return nil
	}
	if si.Pool == "" {
		return ErrMultiClusterServiceRequiresPool
	}
	_, err := servicemanager.Pool.FindByName(ctx, si.Pool)
	if err != nil {
		return err
	}

	poolAllowedServices, err := servicemanager.Pool.Services(ctx, si.Pool)
	if err != nil {
		return err
	}

	if !hasString(poolAllowedServices, s.Name) {
		return ErrMultiClusterViolatingConstraint
	}

	return nil
}

func hasString(slice []string, element string) bool {
	for _, e := range slice {
		if e == element {
			return true
		}
	}
	return false
}
