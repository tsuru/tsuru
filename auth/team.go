// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
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

type ErrTeamStillUsed struct {
	Apps             []string
	ServiceInstances []string
}

type teamService struct {
	storage storage.TeamStorage
}

func (e *ErrTeamStillUsed) Error() string {
	if len(e.Apps) > 0 {
		return fmt.Sprintf("Apps: %s", strings.Join(e.Apps, ", "))
	}
	return fmt.Sprintf("Service instances: %s", strings.Join(e.ServiceInstances, ", "))
}

func (t *teamService) Insert(team authTypes.Team) error {
	return t.storage.Insert(team)
}

func (t *teamService) FindAll() ([]authTypes.Team, error) {
	return t.storage.FindAll()
}

func (t *teamService) FindByName(name string) (*authTypes.Team, error) {
	return t.storage.FindByName(name)
}

func (t *teamService) FindByNames(names []string) ([]authTypes.Team, error) {
	return t.storage.FindByNames(names)
}

func (t *teamService) Delete(team authTypes.Team) error {
	return t.storage.Delete(team)
}

func teamStorage() storage.TeamStorage {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.TeamStorage
}

func validateTeam(t authTypes.Team) error {
	if !teamNameRegexp.MatchString(t.Name) {
		return authTypes.ErrInvalidTeamName
	}
	return nil
}

func TeamService() authTypes.TeamService {
	if ts == nil {
		ts = &teamService{
			storage: teamStorage(),
		}
	}
	return ts
}

// CreateTeam creates a team and add users to this team.
func CreateTeam(name string, user *User) error {
	if user == nil {
		return errors.New("user cannot be null")
	}
	name = strings.TrimSpace(name)
	team := authTypes.Team{
		Name:         name,
		CreatingUser: user.Email,
	}
	if err := validateTeam(team); err != nil {
		return err
	}
	err := TeamService().Insert(team)
	if err != nil {
		return err
	}
	err = user.AddRolesForEvent(permission.RoleEventTeamCreate, name)
	if err != nil {
		log.Errorf("unable to add default roles during team %q creation for %q: %s", name, user.Email, err)
	}
	return nil
}

// GetTeam finds a team by name.
func GetTeam(name string) (*authTypes.Team, error) {
	return TeamService().FindByName(name)
}

// GetTeamsNames maps teams to a list of team names.
func GetTeamsNames(teams []authTypes.Team) []string {
	tn := make([]string, len(teams))
	for i, t := range teams {
		tn[i] = t.Name
	}
	return tn
}

func RemoveTeam(teamName string) error {
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
		return &ErrTeamStillUsed{Apps: apps}
	}
	var serviceInstances []string
	err = conn.ServiceInstances().Find(bson.M{"teams": teamName}).Distinct("name", &serviceInstances)
	if err != nil {
		return err
	}
	if len(serviceInstances) > 0 {
		return &ErrTeamStillUsed{ServiceInstances: serviceInstances}
	}
	return TeamService().Delete(authTypes.Team{Name: teamName})
}

func ListTeams() ([]authTypes.Team, error) {
	return TeamService().FindAll()
}
