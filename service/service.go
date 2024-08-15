// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Service struct {
	Name         string `bson:"_id"`
	Username     string
	Password     string
	Endpoint     map[string]string
	OwnerTeams   []string `bson:"owner_teams"`
	Teams        []string
	Doc          string
	IsRestricted bool `bson:"is_restricted"`
	// IsMultiCluster indicates whether Service Instances (children of this Service)
	// run within the user's Cluster (same pool of Tsuru Apps). When enabled, creating
	// a Service Instance must require a valid Pool.
	//
	// This field is immutable (after creating Service).
	IsMultiCluster bool `bson:"is_multi_cluster"`
}

type BindAppParameters map[string]interface{}

type ProxyOpts struct {
	Instance  *ServiceInstance
	Path      string
	Event     *event.Event
	RequestID string
	Writer    http.ResponseWriter
	Request   *http.Request
}

// TODO: use requestID inside the context
type ServiceClient interface {
	Create(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Update(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Destroy(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	BindApp(ctx context.Context, instance *ServiceInstance, app bind.App, params BindAppParameters, evt *event.Event, requestID string) (map[string]string, error)
	BindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) (map[string]string, error)
	UnbindApp(ctx context.Context, instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error
	UnbindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) error
	Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error)
	Info(ctx context.Context, instance *ServiceInstance, requestID string) ([]map[string]string, error)
	Plans(ctx context.Context, pool, requestID string) ([]Plan, error)
	Proxy(ctx context.Context, opts *ProxyOpts) error
}

var (
	ErrServiceAlreadyExists = errors.New("Service already exists.")
	ErrServiceNotFound      = errors.New("Service not found.")
	ErrMissingPool          = errors.New("Missing pool")

	schemeRegexp = regexp.MustCompile("^https?://")
)

func Get(ctx context.Context, service string) (Service, error) {
	if isBrokeredService(service) {
		return getBrokeredService(ctx, service)
	}
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return Service{}, err
	}
	var s Service
	if err := collection.FindOne(ctx, mongoBSON.M{"_id": service}).Decode(&s); err != nil {
		if err == mongo.ErrNoDocuments {
			return Service{}, ErrServiceNotFound
		}
		return Service{}, err
	}
	return s, nil
}

func Create(ctx context.Context, s Service) error {
	if err := s.validate(ctx, false); err != nil {
		return err
	}
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	n, err := collection.CountDocuments(ctx, mongoBSON.M{"_id": s.Name})
	if err != nil {
		return err
	}
	if n != 0 {
		return ErrServiceAlreadyExists
	}
	_, err = collection.InsertOne(ctx, s)

	return err
}

func Update(ctx context.Context, s Service) error {
	if err := s.validate(ctx, true); err != nil {
		return err
	}
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": s.Name}, s)
	if err == mongo.ErrNoDocuments {
		return ErrServiceNotFound
	}
	return err
}

func Delete(ctx context.Context, s Service) error {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": s.Name})
	if err == mongo.ErrNoDocuments {
		return ErrServiceNotFound
	}
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return ErrServiceNotFound
	}

	return nil
}

func GetServices(ctx context.Context) ([]Service, error) {
	return getServicesByFilter(ctx, nil)
}

func GetServicesByTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter mongoBSON.M
	if teams != nil || services != nil {
		orFilter := []mongoBSON.M{
			{"is_restricted": false},
		}

		if teams != nil {
			orFilter = append(orFilter, mongoBSON.M{"teams": mongoBSON.M{"$in": teams}})
		}
		if services != nil {
			orFilter = append(orFilter, mongoBSON.M{"_id": mongoBSON.M{"$in": services}})
		}

		if len(orFilter) > 1 {
			filter = mongoBSON.M{
				"$or": orFilter,
			}
		} else if len(orFilter) == 1 {
			filter = orFilter[0]
		}
	}
	return getServicesByFilter(ctx, filter)
}

func GetServicesByOwnerTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter mongoBSON.M
	if teams != nil || services != nil {
		orFilter := []mongoBSON.M{}

		if teams != nil {
			orFilter = append(orFilter, mongoBSON.M{"owner_teams": mongoBSON.M{"$in": teams}})
		}
		if services != nil {
			orFilter = append(orFilter, mongoBSON.M{"_id": mongoBSON.M{"$in": services}})
		}

		if len(orFilter) > 1 {
			filter = mongoBSON.M{
				"$or": orFilter,
			}
		} else if len(orFilter) == 1 {
			filter = orFilter[0]
		}
	}
	return getServicesByFilter(ctx, filter)
}

func RenameServiceTeam(ctx context.Context, oldName, newName string) error {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return err
	}

	models := []mongo.WriteModel{}

	for _, field := range []string{"owner_teams", "teams"} {
		models = append(models,
			mongo.NewUpdateManyModel().
				SetFilter(mongoBSON.M{field: oldName}).
				SetUpdate(mongoBSON.M{"$push": mongoBSON.M{field: newName}}),

			mongo.NewUpdateManyModel().
				SetFilter(mongoBSON.M{field: oldName}).
				SetUpdate(mongoBSON.M{"$pull": mongoBSON.M{field: oldName}}),
		)
	}

	_, err = collection.BulkWrite(ctx, models)
	if err != nil {
		return err
	}

	return nil
}

func getServicesByFilter(ctx context.Context, filter mongoBSON.M) ([]Service, error) {
	collection, err := storagev2.ServicesCollection()
	if err != nil {
		return nil, err
	}
	if filter == nil {
		filter = mongoBSON.M{}
	}
	var services []Service
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &services)
	if err != nil {
		return nil, err
	}
	brokerServices, err := getBrokeredServices(ctx)
	if err != nil {
		return nil, err
	}
	return append(services, brokerServices...), err
}

func (s *Service) HasTeam(team *authTypes.Team) bool {
	return s.findTeam(team) > -1
}

func (s *Service) GrantAccess(team *authTypes.Team) error {
	if s.HasTeam(team) {
		return errors.New("This team already has access to this service")
	}
	s.Teams = append(s.Teams, team.Name)
	return nil
}

func (s *Service) RevokeAccess(team *authTypes.Team) error {
	index := s.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this service")
	}
	copy(s.Teams[index:], s.Teams[index+1:])
	s.Teams = s.Teams[:len(s.Teams)-1]
	return nil
}

func (s *Service) getUsername() string {
	if s.Username != "" {
		return s.Username
	}
	return s.Name
}

func (s *Service) findTeam(team *authTypes.Team) int {
	for i, t := range s.Teams {
		if team.Name == t {
			return i
		}
	}
	return -1
}

func endpointNameForPool(ctx context.Context, pool string) (string, error) {
	if pool == "" {
		return "", nil
	}
	p, err := servicemanager.Pool.FindByName(ctx, pool)
	if err != nil {
		return "", err
	}
	c, err := servicemanager.Cluster.FindByPool(ctx, p.Provisioner, p.Name)
	if err != nil {
		if err == provTypes.ErrNoCluster {
			return "", nil
		}
		return "", err
	}
	return c.Name, nil
}

func (s *Service) getClientForPool(ctx context.Context, pool string) (ServiceClient, error) {
	var cli ServiceClient
	poolEndpoint, err := endpointNameForPool(ctx, pool)
	if err != nil {
		return cli, err
	}
	var endpoints []string
	if poolEndpoint != "" {
		endpoints = []string{poolEndpoint, "production"}
	} else {
		endpoints = []string{"production"}
	}
	return s.getClient(endpoints...)
}

func (s *Service) getClient(endpoints ...string) (ServiceClient, error) {
	if isBrokeredService(s.Name) {
		return newBrokeredServiceClient(s.Name)
	}
	var err error
	for _, endpoint := range endpoints {
		if e, ok := s.Endpoint[endpoint]; ok {
			if p := schemeRegexp.MatchString(e); !p {
				e = "http://" + e
			}
			cli := &endpointClient{serviceName: s.Name, endpoint: e, username: s.getUsername(), password: s.Password}
			return cli, nil
		} else {
			err = errors.New("Unknown endpoint: " + endpoint)
		}
	}
	return nil, err
}

func (s *Service) validate(ctx context.Context, skipName bool) (err error) {
	defer func() {
		if err != nil {
			err = &tsuruErrors.ValidationError{Message: err.Error()}
		}
	}()
	if s.Name == "" {
		return fmt.Errorf("Service id is required")
	}
	if isBrokeredService(s.Name) {
		return fmt.Errorf("Brokered services are not managed.")
	}
	if !skipName && !validation.ValidateName(s.Name) {
		return fmt.Errorf("Invalid service id, should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.")
	}
	if s.Password == "" {
		return fmt.Errorf("Service password is required")
	}
	if len(s.Endpoint) == 0 {
		return fmt.Errorf("At least one endpoint is required")
	}
	return s.validateOwnerTeams(ctx)
}

func (s *Service) validateOwnerTeams(ctx context.Context) error {
	if len(s.OwnerTeams) == 0 {
		return fmt.Errorf("At least one service team owner is required")
	}
	teams, err := servicemanager.Team.FindByNames(ctx, s.OwnerTeams)
	if err != nil {
		return nil
	}
	if len(teams) != len(s.OwnerTeams) {
		return fmt.Errorf("Team owner doesn't exist")
	}
	return nil
}

func getServicesNames(services []Service) []string {
	sNames := make([]string, len(services))
	for i, s := range services {
		sNames[i] = s.Name
	}
	return sNames
}

type ServiceModel struct {
	Service          string            `json:"service"`
	Instances        []string          `json:"instances"`
	Plans            []string          `json:"plans"`
	ServiceInstances []ServiceInstance `json:"service_instances"`
}

// Proxy is a proxy between tsuru and the service.
// This method allow customized service methods.
func Proxy(ctx context.Context, service *Service, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	endpoint, err := service.getClient("production")
	if err != nil {
		return err
	}
	return endpoint.Proxy(ctx, &ProxyOpts{
		Path:      path,
		Event:     evt,
		RequestID: requestID,
		Writer:    w,
		Request:   r,
	})
}
