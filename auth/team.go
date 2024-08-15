// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// for some compatibility reasons the name of team must be compliant on some cloud providers
// GCP: https://cloud.google.com/compute/docs/labeling-resources#requirements
//
//	Keys have a minimum length of 1 character and a maximum length of 63 characters, and cannot be empty. Values can be empty, and have a maximum length of 63 characters.
//	Keys and values can contain only lowercase letters, numeric characters, underscores, and dashes. All characters must use UTF-8 encoding, and international characters are allowed. Keys must start with a lowercase letter or international character.
var teamNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9_\-]{1,62}$`)

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
	q, err := startingAppQuota()
	if err != nil {
		return err
	}
	team := authTypes.Team{
		Name:         strings.TrimSpace(name),
		CreatingUser: user.Email,
		Tags:         processTags(tags),
		Quota:        q,
	}
	if err = t.validate(team); err != nil {
		return err
	}
	err = t.storage.Insert(ctx, team)
	if err != nil {
		return err
	}
	u := User(*user)
	err = u.AddRolesForEvent(ctx, permTypes.RoleEventTeamCreate, team.Name)
	if err != nil {
		log.Errorf("unable to add default roles during team %q creation for %q: %s", team.Name, user.Email, err)
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
	appsCollection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	result, err := appsCollection.Distinct(ctx, "name", mongoBSON.M{"teams": teamName})
	if err != nil {
		return err
	}
	if len(result) > 0 {
		var apps []string

		for _, app := range result {
			appStr, ok := app.(string)
			if ok {
				apps = append(apps, appStr)
			}
		}

		return &authTypes.ErrTeamStillUsed{Apps: apps}
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	if err != nil {
		return err
	}

	cursor, err := serviceInstancesCollection.Find(ctx, mongoBSON.M{"teams": teamName}, &options.FindOptions{
		Projection: mongoBSON.M{"name": 1, "service_name": 1},
	})
	if err != nil {
		return err
	}

	type serviceInstance struct {
		Name        string `bson:"name"`
		ServiceName string `bson:"service_name"`
	}

	var (
		serviceInstanceNames []string
		serviceInstances     []serviceInstance
	)

	err = cursor.All(ctx, &serviceInstances)
	if err != nil {
		return err
	}

	for _, si := range serviceInstances {
		serviceInstanceNames = append(serviceInstanceNames, si.ServiceName+"/"+si.Name)
	}

	if len(serviceInstanceNames) > 0 {
		return &authTypes.ErrTeamStillUsed{ServiceInstances: serviceInstanceNames}
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

func startingAppQuota() (quota.Quota, error) {
	limit, err := config.GetInt("quota:apps-per-team")
	if errors.Is(err, config.ErrKeyNotFound{Key: "quota:apps-per-team"}) {
		return quota.UnlimitedQuota, nil // no quota defined in tsurud.yaml, returning unlimited quota
	}
	if err != nil {
		return quota.Quota{}, err
	}
	return quota.Quota{Limit: limit}, nil
}
