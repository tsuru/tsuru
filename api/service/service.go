package service

import (
	"errors"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
)

type Service struct {
	ServiceTypeId bson.ObjectId `bson:"service_type_id"`
	Name          string
	Teams         []auth.Team
}

func (s *Service) Get() error {
	query := bson.M{"name": s.Name}
	return db.Session.Services().Find(query).One(&s)
}

func (s *Service) All() []Service {
	var result []Service
	db.Session.Services().Find(nil).All(&result)
	return result
}

func (s *Service) Create() error {
	err := db.Session.Services().Insert(s)
	if err != nil {
		return err
	}
	u := unit.Unit{Name: s.Name, Type: "mysql"}
	return u.Create()
}

func (s *Service) Delete() error {
	err := db.Session.Services().Remove(s)
	if err != nil {
		return err
	}
	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Charm}
	return u.Destroy()
}

func (s *Service) Bind(a *app.App) error {
	sa := ServiceApp{ServiceName: s.Name, AppName: a.Name}
	return sa.Create()
}

func (s *Service) Unbind(a *app.App) error {
	sa := ServiceApp{ServiceName: s.Name, AppName: a.Name}
	return sa.Delete()
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}

func (s *Service) findTeam(team *auth.Team) int {
	for i, t := range s.Teams {
		if team.Name == t.Name {
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
	s.Teams = append(s.Teams, *team)
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

func (s *Service) CheckUserAccess(user *auth.User) bool {
	for _, team := range s.Teams {
		if team.ContainsUser(user) {
			return true
		}
	}
	return false
}
