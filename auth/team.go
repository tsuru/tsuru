// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

type ErrTeamStillUsed struct {
	Apps             []string
	ServiceInstances []string
}

func TeamService() authTypes.TeamService {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.TeamService
}

func (e *ErrTeamStillUsed) Error() string {
	if len(e.Apps) > 0 {
		return fmt.Sprintf("Apps: %s", strings.Join(e.Apps, ", "))
	}
	return fmt.Sprintf("Service instances: %s", strings.Join(e.ServiceInstances, ", "))
}

func validateTeam(t authTypes.Team) error {
	if !validation.ValidateName(t.Name) {
		return authTypes.ErrInvalidTeamName
	}
	return nil
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

// GetTeam find a team by name.
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
