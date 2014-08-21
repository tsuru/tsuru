// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"regexp"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2/bson"
)

type Service struct {
	Name         string `bson:"_id"`
	Password     string
	Endpoint     map[string]string
	OwnerTeams   []string `bson:"owner_teams"`
	Teams        []string
	Doc          string
	IsRestricted bool `bson:"is_restricted"`
}

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
		cli = &Client{endpoint: e, username: s.Name, password: s.Password}
	} else {
		err = errors.New("Unknown endpoint: " + endpoint)
	}
	return
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

func GetServicesByTeamKindAndNoRestriction(teamKind string, u *auth.User) ([]Service, error) {
	teams, err := u.Teams()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{"$or": []bson.M{
		{teamKind: bson.M{"$in": teamsNames}},
		{"is_restricted": false},
	}}
	var services []Service
	err = conn.Services().Find(q).Select(bson.M{"name": 1}).All(&services)
	return services, err
}

func GetServicesByOwnerTeams(teamKind string, u *auth.User) ([]Service, error) {
	teams, err := u.Teams()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	teamsNames := auth.GetTeamsNames(teams)
	q := bson.M{teamKind: bson.M{"$in": teamsNames}}
	var services []Service
	err = conn.Services().Find(q).All(&services)
	return services, err
}

type ServiceModel struct {
	Service   string   `json:"service"`
	Instances []string `json:"instances"`
}
