// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/types"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
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
}

var (
	ErrServiceAlreadyExists = errors.New("Service already exists.")
)

func (s *Service) Get() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	query := bson.M{"_id": s.Name}
	return conn.Services().Find(query).One(&s)
}

func (s *Service) Create() error {
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

func (s *Service) Update() error {
	if err := s.validate(true); err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Services().Update(bson.M{"_id": s.Name}, s)
}

func (s *Service) Delete() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Services().RemoveAll(bson.M{"_id": s.Name})
	return err
}

func (s *Service) getClient(endpoint string) (cli *Client, err error) {
	if e, ok := s.Endpoint[endpoint]; ok {
		if p, _ := regexp.MatchString("^https?://", e); !p {
			e = "http://" + e
		}
		cli = &Client{serviceName: s.Name, endpoint: e, username: s.GetUsername(), password: s.Password}
	} else {
		err = errors.New("Unknown endpoint: " + endpoint)
	}
	return
}

func (s *Service) GetUsername() string {
	if s.Username != "" {
		return s.Username
	}
	return s.Name
}

func (s *Service) findTeam(team *types.Team) int {
	for i, t := range s.Teams {
		if team.Name == t {
			return i
		}
	}
	return -1
}

func (s *Service) HasTeam(team *types.Team) bool {
	return s.findTeam(team) > -1
}

func (s *Service) GrantAccess(team *types.Team) error {
	if s.HasTeam(team) {
		return errors.New("This team already has access to this service")
	}
	s.Teams = append(s.Teams, team.Name)
	return nil
}

func (s *Service) RevokeAccess(team *types.Team) error {
	index := s.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this service")
	}
	copy(s.Teams[index:], s.Teams[index+1:])
	s.Teams = s.Teams[:len(s.Teams)-1]
	return nil
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
	if !skipName && !validation.ValidateName(s.Name) {
		return fmt.Errorf("Invalid service id, should have at most 63 " +
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
	teams, err := auth.TeamService().FindByNames(s.OwnerTeams)
	if err != nil {
		return nil
	}
	if len(teams) != len(s.OwnerTeams) {
		return fmt.Errorf("Team owner doesn't exist")
	}
	return nil
}

func GetServicesNames(services []Service) []string {
	sNames := make([]string, len(services))
	for i, s := range services {
		sNames[i] = s.Name
	}
	return sNames
}

func GetServicesByFilter(filter bson.M) ([]Service, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var services []Service
	err = conn.Services().Find(filter).All(&services)
	return services, err
}

func GetServicesByTeamsAndServices(teams []string, services []string) ([]Service, error) {
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
	return GetServicesByFilter(filter)
}

func GetServicesByOwnerTeamsAndServices(teams []string, services []string) ([]Service, error) {
	var filter bson.M
	if teams != nil || services != nil {
		filter = bson.M{
			"$or": []bson.M{
				{"owner_teams": bson.M{"$in": teams}},
				{"_id": bson.M{"$in": services}},
			},
		}
	}
	return GetServicesByFilter(filter)
}

type ServiceInstanceModel struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type ServiceModel struct {
	Service          string                 `json:"service"`
	Instances        []string               `json:"instances"`
	Plans            []string               `json:"plans"`
	ServiceInstances []ServiceInstanceModel `json:"service_instances"`
}

// Proxy is a proxy between tsuru and the service.
// This method allow customized service methods.
func Proxy(service *Service, path string, w http.ResponseWriter, r *http.Request) error {
	endpoint, err := service.getClient("production")
	if err != nil {
		return err
	}
	return endpoint.Proxy(path, w, r)
}

func RenameServiceTeam(oldName, newName string) error {
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
