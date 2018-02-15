// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"time"

	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/auth"
)

type TeamTokenService struct{}

type teamToken struct {
	Token        string
	CreatedAt    time.Time  `bson:"created_at"`
	ExpiresAt    *time.Time `bson:"expires_at"`
	LastAccess   *time.Time `bson:"last_access"`
	CreatorEmail string     `bson:"creator_email"`
	AppName      string     `bson:"app"`
	Teams        []string
	Roles        []string `bson:",omitempty"`
}

var _ auth.TeamTokenService = &TeamTokenService{}

func appTokensCollection(conn *db.Storage) *dbStorage.Collection {
	c := conn.Collection("app_tokens")
	c.EnsureIndex(mgo.Index{Key: []string{"token"}, Unique: true, Background: true})
	return c
}

func (s *TeamTokenService) Insert(t auth.TeamToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = appTokensCollection(conn).Insert(teamToken(t))
	if mgo.IsDup(err) {
		return auth.ErrTeamTokenAlreadyExists
	}
	return err
}

func (s *TeamTokenService) FindByToken(token string) (*auth.TeamToken, error) {
	results, err := s.findByQuery(bson.M{"token": token})
	if err != nil {
		if err == mgo.ErrNotFound {
			err = auth.ErrTeamTokenNotFound
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, auth.ErrTeamTokenNotFound
	}
	return &results[0], nil
}

func (s *TeamTokenService) FindByTeam(teamName string) ([]auth.TeamToken, error) {
	return s.findByQuery(bson.M{"teams": bson.M{"$in": []string{teamName}}})
}

func (s *TeamTokenService) findByQuery(query bson.M) ([]auth.TeamToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var tokens []teamToken
	err = appTokensCollection(conn).Find(query).All(&tokens)
	if err != nil {
		return nil, err
	}
	authTeams := make([]auth.TeamToken, len(tokens))
	for i, t := range tokens {
		authTeams[i] = auth.TeamToken(t)
	}
	return authTeams, nil
}

func (s *TeamTokenService) Authenticate(token string) (*auth.TeamToken, error) {
	teamToken, err := s.FindByToken(token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if teamToken.ExpiresAt != nil && teamToken.ExpiresAt.Before(now) {
		return nil, auth.ErrTeamTokenExpired
	}
	teamToken.LastAccess = &now
	err = s.update(*teamToken, bson.M{"last_access": teamToken.LastAccess})
	if err != nil {
		return nil, err
	}
	return teamToken, nil
}

func (s *TeamTokenService) update(teamToken auth.TeamToken, query bson.M) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = appTokensCollection(conn).Update(bson.M{"token": teamToken.Token}, query)
	if err == mgo.ErrNotFound {
		return auth.ErrTeamTokenNotFound
	}
	return err
}

func (s *TeamTokenService) AddTeams(t auth.TeamToken, teams ...string) error {
	return s.update(t, bson.M{
		"$addToSet": bson.M{
			"teams": bson.M{"$each": teams},
		},
	})
}

func (s *TeamTokenService) RemoveTeams(t auth.TeamToken, teams ...string) error {
	if len(teams) == 0 {
		return nil
	}
	return s.update(t, bson.M{
		"$pull": bson.M{
			"teams": bson.M{"$in": teams},
		},
	})
}

func (s *TeamTokenService) AddRoles(t auth.TeamToken, newRoles ...string) error {
	return s.update(t, bson.M{
		"$addToSet": bson.M{
			"roles": bson.M{"$each": newRoles},
		},
	})
}

func (s *TeamTokenService) RemoveRoles(t auth.TeamToken, newRoles ...string) error {
	if len(newRoles) == 0 {
		return nil
	}
	return s.update(t, bson.M{
		"$pull": bson.M{
			"roles": bson.M{"$in": newRoles},
		},
	})
}

func (s *TeamTokenService) Delete(t auth.TeamToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = appTokensCollection(conn).Remove(bson.M{"token": t.Token})
	if err == mgo.ErrNotFound {
		return auth.ErrTeamTokenNotFound
	}
	return err
}
