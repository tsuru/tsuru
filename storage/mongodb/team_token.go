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

type teamTokenStorage struct{}

type teamToken struct {
	Token        string
	TokenID      string `bson:"token_id"`
	Description  string
	CreatedAt    time.Time `bson:"created_at"`
	ExpiresAt    time.Time `bson:"expires_at,omitempty"`
	LastAccess   time.Time `bson:"last_access,omitempty"`
	CreatorEmail string    `bson:"creator_email"`
	Team         string
	Roles        []auth.RoleInstance `bson:",omitempty"`
}

var _ auth.TeamTokenStorage = &teamTokenStorage{}

func teamTokensCollection(conn *db.Storage) *dbStorage.Collection {
	c := conn.Collection("team_tokens")
	c.EnsureIndex(mgo.Index{Key: []string{"token"}, Unique: true})
	c.EnsureIndex(mgo.Index{Key: []string{"token_id"}, Unique: true})
	return c
}

func (s *teamTokenStorage) Insert(t auth.TeamToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamTokensCollection(conn).Insert(teamToken(t))
	if mgo.IsDup(err) {
		return auth.ErrTeamTokenAlreadyExists
	}
	return err
}

func (s *teamTokenStorage) findOne(query bson.M) (*auth.TeamToken, error) {
	results, err := s.findByQuery(query)
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

func (s *teamTokenStorage) FindByToken(token string) (*auth.TeamToken, error) {
	return s.findOne(bson.M{"token": token})
}

func (s *teamTokenStorage) FindByTokenID(tokenID string) (*auth.TeamToken, error) {
	return s.findOne(bson.M{"token_id": tokenID})
}

func (s *teamTokenStorage) FindByTeams(teamNames []string) ([]auth.TeamToken, error) {
	query := bson.M{}
	if teamNames != nil {
		query["team"] = bson.M{"$in": teamNames}
	}
	return s.findByQuery(query)
}

func (s *teamTokenStorage) findByQuery(query bson.M) ([]auth.TeamToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var tokens []teamToken
	err = teamTokensCollection(conn).Find(query).All(&tokens)
	if err != nil {
		return nil, err
	}
	authTeams := make([]auth.TeamToken, len(tokens))
	for i, t := range tokens {
		authTeams[i] = auth.TeamToken(t)
	}
	return authTeams, nil
}

func (s *teamTokenStorage) UpdateLastAccess(token string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamTokensCollection(conn).Update(bson.M{
		"token": token,
	}, bson.M{
		"$set": bson.M{"last_access": time.Now().UTC()},
	})
	if err == mgo.ErrNotFound {
		err = auth.ErrTeamTokenNotFound
	}
	return err
}

func (s *teamTokenStorage) Update(token auth.TeamToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamTokensCollection(conn).Update(bson.M{"token_id": token.TokenID}, teamToken(token))
	if err == mgo.ErrNotFound {
		err = auth.ErrTeamTokenNotFound
	}
	return err
}

func (s *teamTokenStorage) Delete(token string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamTokensCollection(conn).Remove(bson.M{"token_id": token})
	if err == mgo.ErrNotFound {
		return auth.ErrTeamTokenNotFound
	}
	return err
}
