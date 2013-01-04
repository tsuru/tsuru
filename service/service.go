// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"strings"
)

type Service struct {
	Name         string `bson:"_id"`
	Endpoint     map[string]string
	OwnerTeams   []string `bson:"owner_teams"`
	Teams        []string
	Status       string
	Doc          string
	IsRestricted bool `bson:"is_restricted"`
}

type ServiceModel struct {
	Service   string
	Instances []string
}

func (s *Service) Get() error {
	query := bson.M{"_id": s.Name, "status": bson.M{"$ne": "deleted"}}
	return db.Session.Services().Find(query).One(&s)
}

func (s *Service) Create() error {
	s.Status = "created"
	return db.Session.Services().Insert(s)
}

func (s *Service) Update() error {
	return db.Session.Services().Update(bson.M{"_id": s.Name}, s)
}

func (s *Service) Delete() error {
	s.Status = "deleted"
	return db.Session.Services().Update(bson.M{"_id": s.Name}, s)
}

func (s *Service) getClient(endpoint string) (cli *Client, err error) {
	if e, ok := s.Endpoint[endpoint]; ok {
		if !strings.HasPrefix(e, "http://") {
			e = "http://" + e
		}
		cli = &Client{endpoint: e}
	} else {
		err = errors.New("Unknown endpoint: " + endpoint)
	}
	return
}

func (s *Service) ProductionEndpoint() *Client {
	cli, _ := s.getClient("production")
	return cli
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

func GetServicesByTeamKindAndNoRestriction(teamKind string, u *auth.User) (services []Service, err error) {
	teams, err := u.Teams()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{"$or": []bson.M{
		{teamKind: bson.M{"$in": teamsNames}},
		{"is_restricted": false},
	},
		"status": bson.M{"$ne": "deleted"},
	}
	err = db.Session.Services().Find(q).Select(bson.M{"name": 1}).All(&services)
	return
}

func GetServicesByOwnerTeams(teamKind string, u *auth.User) (services []Service, err error) {
	teams, err := u.Teams()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{teamKind: bson.M{"$in": teamsNames}, "status": bson.M{"$ne": "deleted"}}
	err = db.Session.Services().Find(q).All(&services)
	return
}
