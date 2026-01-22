// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	ErrInvalidProxyPath                         = errors.New("invalid proxy path")
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

	// ForceRemove indicates whether service instance should be removed even the
	// related call to service API fails.
	ForceRemove bool `bson:"-" json:"-"`
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
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return err
	}

	_, err = collection.DeleteOne(ctx, mongoBSON.M{"name": si.Name, "service_name": si.ServiceName})
	return err
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

func (si *ServiceInstance) updateData(ctx context.Context, update mongoBSON.M) error {
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": si.Name, "service_name": si.ServiceName}, update)
	return err
}

// BindApp makes the bind between the service instance and an app.
func (si *ServiceInstance) BindApp(ctx context.Context, app *appTypes.App, params BindAppParameters, shouldRestart bool, writer io.Writer, evt *event.Event, requestID string) error {
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
	App         *appTypes.App
	Restart     bool
	ForceRemove bool
	Event       *event.Event
	RequestID   string
}

// UnbindApp makes the unbind between the service instance and an app.
func (si *ServiceInstance) UnbindApp(ctx context.Context, unbindArgs UnbindAppArgs) error {
	if si.FindApp(unbindArgs.App.Name) == -1 {
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
	return si.updateData(ctx, mongoBSON.M{"$addToSet": mongoBSON.M{"teams": team.Name}})
}

func (si *ServiceInstance) Revoke(ctx context.Context, teamName string) error {
	if teamName == si.TeamOwner {
		return ErrRevokeInstanceTeamOwnerAccess
	}
	team, err := servicemanager.Team.FindByName(ctx, teamName)
	if err != nil {
		return err
	}
	return si.updateData(ctx, mongoBSON.M{"$pull": mongoBSON.M{"teams": team.Name}})
}

func genericServiceInstancesFilter(services interface{}, teams []string) mongoBSON.M {
	query := mongoBSON.M{}
	if len(teams) != 0 {
		query["teams"] = mongoBSON.M{"$in": teams}
	}
	if v, ok := services.([]Service); ok {
		names := getServicesNames(v)
		query["service_name"] = mongoBSON.M{"$in": names}
	}
	if v, ok := services.(Service); ok {
		query["service_name"] = v.Name
	}
	return query
}

func validateServiceInstance(ctx context.Context, si ServiceInstance, s *Service) error {
	err := validateServiceInstanceName(ctx, s.Name, si.Name)
	if err != nil {
		return err
	}
	err = validateServiceInstanceTeamOwner(ctx, si)
	if err != nil {
		return err
	}
	return validateMultiCluster(ctx, s, si)
}

func validateServiceInstanceName(ctx context.Context, service, instance string) error {
	if !instanceNameRegexp.MatchString(instance) {
		return ErrInvalidInstanceName
	}
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil
	}
	query := mongoBSON.M{"name": instance, "service_name": service}
	length, err := collection.CountDocuments(ctx, query)
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

func GetServiceInstancesByServices(ctx context.Context, services []Service, tags []string) ([]ServiceInstance, error) {
	var instances []ServiceInstance
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil, err
	}
	query := genericServiceInstancesFilter(services, []string{})
	if len(tags) > 0 {
		query["tags"] = mongoBSON.M{"$all": tags}
	}
	f := mongoBSON.M{"name": 1, "service_name": 1, "tags": 1}

	opts := options.Find().SetProjection(f)
	cursor, err := collection.Find(ctx, query, opts)
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &instances)
	if err != nil {
		return nil, err
	}
	return instances, err
}

func GetServicesInstancesByTeamsAndNames(ctx context.Context, teams []string, names []string, appName, serviceName string, tags []string) ([]ServiceInstance, error) {
	filter := mongoBSON.M{}
	if teams != nil || names != nil {
		orConditions := []mongoBSON.M{}

		if teams != nil {
			orConditions = append(orConditions, mongoBSON.M{"teams": mongoBSON.M{"$in": teams}})
		}

		if names != nil {
			orConditions = append(orConditions, mongoBSON.M{"name": mongoBSON.M{"$in": names}})
		}

		filter = mongoBSON.M{"$or": orConditions}
	}
	if appName != "" {
		filter["apps"] = appName
	}
	if serviceName != "" {
		filter["service_name"] = serviceName
	}
	if len(tags) > 0 {
		filter["tags"] = mongoBSON.M{"$all": tags}
	}
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil, err
	}
	var instances []ServiceInstance
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &instances)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func GetServiceInstance(ctx context.Context, serviceName string, instanceName string) (*ServiceInstance, error) {
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil, err
	}
	var instance ServiceInstance
	err = collection.FindOne(ctx, mongoBSON.M{"name": instanceName, "service_name": serviceName}).Decode(&instance)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrServiceInstanceNotFound
		}

		return nil, err
	}
	return &instance, nil
}

func GetServiceInstancesBoundToApp(ctx context.Context, appName string) ([]ServiceInstance, error) {
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil, err
	}
	var instances []ServiceInstance
	q := mongoBSON.M{"apps": mongoBSON.M{"$in": []string{appName}}}
	cursor, err := collection.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &instances)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func GetServiceInstancesBoundToJob(ctx context.Context, jobName string) ([]ServiceInstance, error) {
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return nil, err
	}
	var instances []ServiceInstance
	q := mongoBSON.M{"jobs": mongoBSON.M{"$in": []string{jobName}}}
	cursor, err := collection.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &instances)
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
	collection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return err
	}

	updates := []mongo.WriteModel{
		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teamowner": oldName}).
			SetUpdate(mongoBSON.M{"$set": mongoBSON.M{"teamowner": newName}}),

		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teams": oldName}).
			SetUpdate(mongoBSON.M{"$addToSet": mongoBSON.M{"teams": newName}}),

		mongo.NewUpdateManyModel().
			SetFilter(mongoBSON.M{"teams": oldName}).
			SetUpdate(mongoBSON.M{"$pull": mongoBSON.M{"teams": oldName}}),
	}

	_, err = collection.BulkWrite(ctx, updates)
	return err
}

// ProxyInstance is a proxy between tsuru and the service instance.
// This method allow customized service instance methods.
func ProxyInstance(ctx context.Context, instance *ServiceInstance, requestPath string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	service, err := Get(ctx, instance.ServiceName)
	if err != nil {
		return err
	}
	endpoint, err := service.getClientForPool(ctx, instance.Pool)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("/resources/%s", instance.GetIdentifier())
	cleanPath := path.Clean("/" + requestPath)
	// Allow empty path or "/" to map to the instance resource
	if cleanPath == "/" || cleanPath == "" {
		cleanPath = prefix
	}
	var relativePath string
	if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/") {
		// Path is within the expected prefix, extract relative part
		relativePath = strings.Trim(strings.TrimPrefix(cleanPath, prefix), "/")
	} else {
		// Path doesn't start with prefix - check for path traversal attempts
		// that try to escape via ".." after the prefix
		if strings.HasPrefix(requestPath, prefix) && cleanPath != requestPath {
			// Original path had prefix but clean path doesn't - likely ".." traversal
			return ErrInvalidProxyPath
		}
		// Otherwise, treat the entire clean path as relative (legacy behavior)
		relativePath = strings.Trim(cleanPath, "/")
	}
	for _, reserved := range reservedProxyPaths {
		if relativePath == reserved && r.Method != "GET" {
			return &tsuruErrors.ValidationError{
				Message: fmt.Sprintf("proxy request %s %q is forbidden", r.Method, relativePath),
			}
		}
	}

	return endpoint.Proxy(ctx, &ProxyOpts{
		Instance:  instance,
		Path:      fmt.Sprintf("%s/%s", prefix, relativePath),
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
