package service

import (
	"errors"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
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

func (s *Service) Log(out []byte) {
	log.Printf(string(out))
}

func (s *Service) Get() error {
	query := bson.M{"_id": s.Name, "status": bson.M{"$ne": "deleted"}}
	return db.Session.Services().Find(query).One(&s)
}

func (s *Service) Create() error {
	s.Status = "created"
	err := db.Session.Services().Insert(s)
	return err
}

func (s *Service) Update() error {
	return db.Session.Services().Update(bson.M{"_id": s.Name}, s)
}

func (s *Service) Delete() error {
	s.Status = "deleted"
	return db.Session.Services().Update(bson.M{"_id": s.Name}, s)
}

func (s *Service) GetClient(endpoint string) (cli *Client, err error) {
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
	last := len(s.Teams) - 1
	s.Teams[index] = s.Teams[last]
	s.Teams = s.Teams[:last]
	return nil
}

func GetServicesNames(services []Service) []string {
	sNames := make([]string, len(services))
	for i, s := range services {
		sNames[i] = s.Name
	}
	return sNames
}
