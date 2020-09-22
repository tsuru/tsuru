// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
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

	ctx context.Context
}

type BindAppParameters map[string]interface{}

// TODO: use requestID inside the context
type ServiceClient interface {
	Create(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Update(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	Destroy(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error
	BindApp(ctx context.Context, instance *ServiceInstance, app bind.App, params BindAppParameters, evt *event.Event, requestID string) (map[string]string, error)
	BindUnit(ctx context.Context, instance *ServiceInstance, app bind.App, unit bind.Unit) error
	UnbindApp(ctx context.Context, instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error
	UnbindUnit(ctx context.Context, instance *ServiceInstance, app bind.App, unit bind.Unit) error
	Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error)
	Info(ctx context.Context, instance *ServiceInstance, requestID string) ([]map[string]string, error)
	Plans(ctx context.Context, requestID string) ([]Plan, error)
	Proxy(ctx context.Context, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error
}

var (
	ErrServiceAlreadyExists = errors.New("Service already exists.")
	ErrServiceNotFound      = errors.New("Service not found.")

	schemeRegexp = regexp.MustCompile("^https?://")
)

func Get(ctx context.Context, service string) (Service, error) {
	if isBrokeredService(service) {
		return getBrokeredService(ctx, service)
	}
	conn, err := db.Conn()
	if err != nil {
		return Service{}, err
	}
	defer conn.Close()
	var s Service
	if err := conn.Services().Find(bson.M{"_id": service}).One(&s); err != nil {
		if err == mgo.ErrNotFound {
			return Service{}, ErrServiceNotFound
		}
		return Service{}, err
	}
	s.ctx = ctx
	return s, nil
}

func Create(s Service) error {
	if err := s.validate(false); err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	n, err := conn.Services().Find(bson.M{"_id": s.Name}).Count()
	if err != nil {
		return err
	}
	if n != 0 {
		return ErrServiceAlreadyExists
	}
	return conn.Services().Insert(s)
}

func Update(s Service) error {
	if err := s.validate(true); err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Services().Update(bson.M{"_id": s.Name}, s)
	if err == mgo.ErrNotFound {
		return ErrServiceNotFound
	}
	return err
}

func Delete(s Service) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Services().RemoveAll(bson.M{"_id": s.Name})
	if err == mgo.ErrNotFound {
		return ErrServiceNotFound
	}
	return err
}

func GetServices(ctx context.Context) ([]Service, error) {
	return getServicesByFilter(ctx, nil)
}

func GetServicesByTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter bson.M
	if teams != nil || services != nil {
		filter = bson.M{
			"$or": []bson.M{
				{"teams": bson.M{"$in": teams}},
				{"_id": bson.M{"$in": services}},
				{"is_restricted": false},
			},
		}
	}
	return getServicesByFilter(ctx, filter)
}

func GetServicesByOwnerTeamsAndServices(ctx context.Context, teams []string, services []string) ([]Service, error) {
	var filter bson.M
	if teams != nil || services != nil {
		filter = bson.M{
			"$or": []bson.M{
				{"owner_teams": bson.M{"$in": teams}},
				{"_id": bson.M{"$in": services}},
			},
		}
	}
	return getServicesByFilter(ctx, filter)
}

func RenameServiceTeam(ctx context.Context, oldName, newName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	fields := []string{"owner_teams", "teams"}
	bulk := conn.Services().Bulk()
	for _, f := range fields {
		bulk.UpdateAll(bson.M{f: oldName}, bson.M{"$push": bson.M{f: newName}})
		bulk.UpdateAll(bson.M{f: oldName}, bson.M{"$pull": bson.M{f: oldName}})
	}
	_, err = bulk.Run()
	return err
}

func getServicesByFilter(ctx context.Context, filter bson.M) ([]Service, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var services []Service
	err = conn.Services().Find(filter).All(&services)
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

func (s *Service) getClient(endpoint string) (cli ServiceClient, err error) {
	if isBrokeredService(s.Name) {
		return newBrokeredServiceClient(s.Name)
	}
	if e, ok := s.Endpoint[endpoint]; ok {
		if p := schemeRegexp.MatchString(e); !p {
			e = "http://" + e
		}
		cli = &endpointClient{serviceName: s.Name, endpoint: e, username: s.getUsername(), password: s.Password}
	} else {
		err = errors.New("Unknown endpoint: " + endpoint)
	}
	return
}

func (s *Service) validate(skipName bool) (err error) {
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
	if endpoint, ok := s.Endpoint["production"]; !ok || endpoint == "" {
		return fmt.Errorf("Service production endpoint is required")
	}
	return s.validateOwnerTeams()
}

func (s *Service) validateOwnerTeams() error {
	if len(s.OwnerTeams) == 0 {
		return fmt.Errorf("At least one service team owner is required")
	}
	teams, err := servicemanager.Team.FindByNames(s.ctx, s.OwnerTeams)
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
	return endpoint.Proxy(ctx, path, evt, requestID, w, r)
}
