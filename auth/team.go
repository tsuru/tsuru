// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrInvalidTeamName = &tsuruErrors.ValidationError{
		Message: "Invalid team name, team name should have at most 63 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter.",
	}
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")
)

type ErrTeamStillUsed struct {
	Apps             []string
	ServiceInstances []string
}

func (e *ErrTeamStillUsed) Error() string {
	if len(e.Apps) > 0 {
		return fmt.Sprintf("Apps: %s", strings.Join(e.Apps, ", "))
	}
	return fmt.Sprintf("Service instances: %s", strings.Join(e.ServiceInstances, ", "))
}

// Team represents a real world team, a team has one creating user and a name.
type Team struct {
	Name         string `bson:"_id" json:"name"`
	CreatingUser string
}

func (t *Team) validate() error {
	if !validation.ValidateName(t.Name) {
		return ErrInvalidTeamName
	}
	return nil
}

// CreateTeam creates a team and add users to this team.
func CreateTeam(name string, user *User) error {
	if user == nil {
		return errors.New("user cannot be null")
	}
	name = strings.TrimSpace(name)
	team := Team{
		Name:         name,
		CreatingUser: user.Email,
	}
	if err := team.validate(); err != nil {
		return err
	}
	err := storage.TeamRepository.Insert(storage.Team(team))
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
func GetTeam(name string) (*Team, error) {
	t, err := storage.TeamRepository.FindByName(name)
	if t == nil {
		return nil, err
	}
	return &Team{Name: t.Name, CreatingUser: t.CreatingUser}, err
}

// GetTeamsNames find teams by a list of team names.
func GetTeamsNames(teams []Team) []string {
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
	err = storage.TeamRepository.Delete(storage.Team{Name: teamName})
	if err == storage.ErrTeamNotFound {
		return ErrTeamNotFound
	}
	return nil
}

func ListTeams() ([]Team, error) {
	t, err := storage.TeamRepository.FindAll()
	if err != nil {
		return nil, err
	}
	teams := make([]Team, len(t))
	for i, team := range t {
		teams[i].Name = team.Name
		teams[i].CreatingUser = team.CreatingUser
	}
	return teams, nil
}
