// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrInvalidTeamName   = errors.New("invalid team name")
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")

	teamNameRegexp = regexp.MustCompile(`^[a-zA-Z][-@_.+\w]+$`)
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

// AllowedApps returns the apps that the team has access.
func (t *Team) AllowedApps() ([]string, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	allowedApps := []map[string]string{}
	query := conn.Apps().Find(bson.M{"teams": t.Name})
	if err := query.Select(bson.M{"name": 1}).All(&allowedApps); err != nil {
		return nil, err
	}
	appNames := make([]string, len(allowedApps))
	for i, app := range allowedApps {
		appNames[i] = app["name"]
	}
	return appNames, nil
}

// CreateTeam creates a team and add users to this team.
func CreateTeam(name string, user *User) error {
	if user == nil {
		return errors.New("user cannot be null")
	}
	name = strings.TrimSpace(name)
	if !isTeamNameValid(name) {
		return ErrInvalidTeamName
	}
	team := Team{
		Name:         name,
		CreatingUser: user.Email,
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Teams().Insert(team)
	if mgo.IsDup(err) {
		return ErrTeamAlreadyExists
	}
	if err != nil {
		return err
	}
	err = user.AddRolesForEvent(permission.RoleEventTeamCreate, name)
	if err != nil {
		log.Errorf("unable to add default roles during team %q creation for %q: %s", name, user.Email, err)
	}
	return nil
}

func isTeamNameValid(name string) bool {
	return teamNameRegexp.MatchString(name)
}

// GetTeam find a team by name.
func GetTeam(name string) (*Team, error) {
	var t Team
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Teams().FindId(name).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = ErrTeamNotFound
		}
		return nil, err
	}
	return &t, nil
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
	err = conn.Teams().RemoveId(teamName)
	if err == mgo.ErrNotFound {
		return ErrTeamNotFound
	}
	return nil
}

func ListTeams() ([]Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []Team
	err = conn.Teams().Find(nil).All(&teams)
	if err != nil {
		return nil, err
	}
	return teams, nil
}
