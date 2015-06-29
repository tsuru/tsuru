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
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrInvalidTeamName   = errors.New("invalid team name")
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrTeamNotFound      = errors.New("team not found")

	teamNameRegexp = regexp.MustCompile(`^[a-zA-Z][-@_.+\w]+$`)
)

// Team represents a real world team, a team has team members (users) and
// a name.
type Team struct {
	Name      string   `bson:"_id" json:"name"`
	Users     []string `json:"users"`
	TeamLeads []string `json:"team_leads"`
}

// ContainsUser checks if the team contains the user.
func (t *Team) ContainsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user {
			return true
		}
	}
	return false
}

// ContainsTeamLead checks if the team contains the team lead.
func (t *Team) ContainsTeamLead(u *User) bool {
	for _, user := range t.TeamLeads {
		if u.Email == user {
			return true
		}
	}
	return false
}

// AddUser adds a user to the team.
func (t *Team) AddUser(u *User) error {
	if t.ContainsUser(u) {
		return fmt.Errorf("User %s is already in the team %s.", u.Email, t.Name)
	}
	t.Users = append(t.Users, u.Email)
	return nil
}

// AddTeamLead adds a team lead to the team.
func (t *Team) AddTeamLead(u *User) error {
	if !t.ContainsUser(u) {
		return fmt.Errorf("User %s must be member of the team %s before he/she can become team lead.", u.Email, t.Name)
	}
	if t.ContainsTeamLead(u) {
		return fmt.Errorf("User %s is already lead of the team %s.", u.Email, t.Name)
	}
	t.TeamLeads = append(t.TeamLeads, u.Email)
	return nil
}

// RemoveUser removes a user from the team.
func (t *Team) RemoveUser(u *User) error {
	index := -1
	for i, user := range t.Users {
		if u.Email == user {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("User %s is not in the team %s.", u.Email, t.Name)
	}

	// If the user is a team lead,
	// let's try removing him from TeamLeads slice first
	if t.ContainsTeamLead(u) {
		if err := t.RemoveTeamLead(u); err != nil {
			return err
		}
	}

	last := len(t.Users) - 1
	if index < last {
		t.Users[index] = t.Users[last]
	}
	t.Users = t.Users[:last]
	return nil
}

// RemoveTeamLead removes user from team leads.
func (t *Team) RemoveTeamLead(u *User) error {
	index := -1
	for i, user := range t.TeamLeads {
		if u.Email == user {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("User %s is not lead of the team %s.", u.Email, t.Name)
	}
	last := len(t.TeamLeads) - 1
	if index < last {
		t.TeamLeads[index] = t.TeamLeads[last]
	}
	t.TeamLeads = t.TeamLeads[:last]
	return nil
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
func CreateTeam(name string, user ...*User) error {
	name = strings.TrimSpace(name)
	if !isTeamNameValid(name) {
		return ErrInvalidTeamName
	}
	team := Team{
		Name:  name,
		Users: make([]string, len(user)),
	}
	for i, u := range user {
		team.Users[i] = u.Email
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
	return err
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

// CheckUserAccess verifies if the user has access to a list
// of teams.
func CheckUserAccess(teamNames []string, u *User) bool {
	q := bson.M{"_id": bson.M{"$in": teamNames}}
	var teams []Team
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err)
		return false
	}
	defer conn.Close()
	conn.Teams().Find(q).All(&teams)
	for _, team := range teams {
		if team.ContainsUser(u) {
			return true
		}
	}
	return false
}
