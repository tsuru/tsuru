// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

var teamNameRegexp = regexp.MustCompile(`^[a-z][-@_.+\w]+$`)
var ts authTypes.TeamService

type teamService struct {
	storage authTypes.TeamStorage
}

func TeamService() authTypes.TeamService {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return &teamService{
		storage: dbDriver.TeamStorage,
	}
}

func (t *teamService) Create(name string, user *authTypes.User) error {
	if user == nil {
		return errors.New("user cannot be null")
	}
	name = strings.TrimSpace(name)
	team := authTypes.Team{
		Name:         name,
		CreatingUser: user.Email,
	}
	if err := t.validate(team); err != nil {
		return err
	}
	err := t.storage.Insert(team)
	if err != nil {
		return err
	}
	u := User(*user)
	err = u.AddRolesForEvent(permission.RoleEventTeamCreate, name)
	if err != nil {
		log.Errorf("unable to add default roles during team %q creation for %q: %s", name, user.Email, err)
	}
	return nil
}

func (t *teamService) List() ([]authTypes.Team, error) {
	return t.storage.FindAll()
}

func (t *teamService) FindByName(name string) (*authTypes.Team, error) {
	return t.storage.FindByName(name)
}

func (t *teamService) FindByNames(names []string) ([]authTypes.Team, error) {
	return t.storage.FindByNames(names)
}

func (t *teamService) Remove(teamName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var apps []string
	err = conn.Apps().Find(bson.M{"teams": teamName}).Distinct("name", &apps)
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		return &authTypes.ErrTeamStillUsed{Apps: apps}
	}
	var serviceInstances []string
	err = conn.ServiceInstances().Find(bson.M{"teams": teamName}).Distinct("name", &serviceInstances)
	if err != nil {
		return err
	}
	if len(serviceInstances) > 0 {
		return &authTypes.ErrTeamStillUsed{ServiceInstances: serviceInstances}
	}
	return t.storage.Delete(authTypes.Team{Name: teamName})
}

func (t *teamService) validate(team authTypes.Team) error {
	if !teamNameRegexp.MatchString(team.Name) {
		return authTypes.ErrInvalidTeamName
	}
	return nil
}
