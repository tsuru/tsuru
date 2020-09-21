// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var teamNameRegexp = regexp.MustCompile(`^[a-z][-@_.+\w]+$`)

type teamService struct {
	storage authTypes.TeamStorage
}

func TeamService() (authTypes.TeamService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &teamService{
		storage: dbDriver.TeamStorage,
	}, nil
}

func (t *teamService) Create(ctx context.Context, name string, tags []string, user *authTypes.User) error {
	if user == nil {
		return errors.New("user cannot be null")
	}
	name = strings.TrimSpace(name)
	team := authTypes.Team{
		Name:         name,
		CreatingUser: user.Email,
		Tags:         processTags(tags),
	}
	if err := t.validate(team); err != nil {
		return err
	}
	err := t.storage.Insert(ctx, team)
	if err != nil {
		return err
	}
	u := User(*user)
	err = u.AddRolesForEvent(permTypes.RoleEventTeamCreate, name)
	if err != nil {
		log.Errorf("unable to add default roles during team %q creation for %q: %s", name, user.Email, err)
	}
	return nil
}

func (t *teamService) Update(ctx context.Context, name string, tags []string) error {
	team, err := t.storage.FindByName(ctx, name)
	if err != nil {
		return err
	}
	team.Tags = processTags(tags)
	return t.storage.Update(ctx, *team)
}

func (t *teamService) List(ctx context.Context) ([]authTypes.Team, error) {
	return t.storage.FindAll(ctx)
}

func (t *teamService) FindByName(ctx context.Context, name string) (*authTypes.Team, error) {
	return t.storage.FindByName(ctx, name)
}

func (t *teamService) FindByNames(ctx context.Context, names []string) ([]authTypes.Team, error) {
	return t.storage.FindByNames(ctx, names)
}

func (t *teamService) Remove(ctx context.Context, teamName string) error {
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
	return t.storage.Delete(ctx, authTypes.Team{Name: teamName})
}

func (t *teamService) validate(team authTypes.Team) error {
	if !teamNameRegexp.MatchString(team.Name) {
		return authTypes.ErrInvalidTeamName
	}
	return nil
}

// processTags removes duplicates and trims spaces from each tag
func processTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	processedTags := []string{}
	usedTags := make(map[string]bool)
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 0 && !usedTags[tag] {
			processedTags = append(processedTags, tag)
			usedTags[tag] = true
		}
	}
	return processedTags
}
