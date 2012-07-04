package service

import (
	"errors"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"labix.org/v2/mgo/bson"
	"strings"
)

const OnNewInstance = "on-new-instance"

type Service struct {
	Name      string `bson:"_id"`
	Endpoint  map[string]string
	Bootstrap map[string]string
	Teams     []string
}

func (s *Service) Log(out []byte) {
	log.Printf(string(out))
}

func (s *Service) Get() error {
	query := bson.M{"_id": s.Name}
	return db.Session.Services().Find(query).One(&s)
}

func (s *Service) All() []Service {
	var result []Service
	db.Session.Services().Find(nil).All(&result)
	return result
}

func (s *Service) Create() error {
	err := db.Session.Services().Insert(s)
	return err
}

func (s *Service) Delete() error {
	err := db.Session.Services().Remove(s)
	return err
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

// func (s *Service) Bind(a *app.App) error {
// 	sa := ServiceInstance{Name: s.Name, Apps: a.Name}
// 	return sa.Create()
// }

// func (s *Service) Unbind(a *app.App) error {
// 	sa := ServiceInstance{Name: s.Name, Apps: a.Name}
// 	return sa.Delete()
// }

func (s *Service) findTeam(team *auth.Team) int {
	for i, t := range s.Teams {
		if team.Name == t {
			return i
		}
	}
	return -1
}

func (s *Service) hasTeam(team *auth.Team) bool {
	return s.findTeam(team) > -1
}

func (s *Service) GrantAccess(team *auth.Team) error {
	if s.hasTeam(team) {
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
