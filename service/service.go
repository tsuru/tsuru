// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
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
		cli = &Client{endpoint: e, username: s.GetUsername(), password: s.Password}
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

func (s *Service) findTeam(team *auth.Team) int {
	for i, t := range s.Teams {
		if team.Name == t {
			return i
		}
	}
	return -1
}

func (s *Service) HasTeam(team *auth.Team) bool {
	return s.findTeam(team) > -1
}

func (s *Service) GrantAccess(team *auth.Team) error {
	if s.HasTeam(team) {
		return errors.New("This team already has access to this service")
	}
	s.Teams = append(s.Teams, team.Name)
	return nil
}

func (s *Service) RevokeAccess(team *auth.Team) error {
	index := s.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this service")
	}
	copy(s.Teams[index:], s.Teams[index+1:])
	s.Teams = s.Teams[:len(s.Teams)-1]
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

type ServiceModel struct {
	Service   string   `json:"service"`
	Instances []string `json:"instances"`
	Plans     []string `json:"plans"`
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
