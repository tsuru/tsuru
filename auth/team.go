// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"regexp"
	"strings"
	"sync"
)

var (
	ErrInvalidTeamName   = errors.New("Invalid team name")
	ErrTeamAlreadyExists = errors.New("Team already exists")

	teamNameRegexp = regexp.MustCompile(`^[a-zA-Z][-@_.+\w\s]+$`)
)

type Team struct {
	Name  string   `bson:"_id" json:"name"`
	Users []string `json:"users"`
}

func (t *Team) ContainsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user {
			return true
		}
	}
	return false
}

func (t *Team) AddUser(u *User) error {
	if t.ContainsUser(u) {
		return fmt.Errorf("User %s is already in the team %s.", u.Email, t.Name)
	}
	t.Users = append(t.Users, u.Email)
	return nil
}

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
	last := len(t.Users) - 1
	if index < last {
		t.Users[index] = t.Users[last]
	}
	t.Users = t.Users[:last]
	return nil
}

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

func GetTeam(name string) (*Team, error) {
	var t Team
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	err = conn.Teams().FindId(name).One(&t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func GetTeamsNames(teams []Team) []string {
	tn := make([]string, len(teams))
	for i, t := range teams {
		tn[i] = t.Name
	}
	return tn
}

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
	var wg sync.WaitGroup
	found := make(chan bool, len(teams)+1)
	for _, team := range teams {
		wg.Add(1)
		go func(t Team) {
			if t.ContainsUser(u) {
				found <- true
			}
			wg.Done()
		}(team)
	}
	go func() {
		wg.Wait()
		found <- false
	}()
	return <-found
}
