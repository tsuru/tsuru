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

type AppTokenService struct{}

type appToken struct {
	Token        string
	Creation     time.Time
	Expires      time.Duration
	LastAccess   time.Time `bson:"last_access"`
	CreatorEmail string    `bson:"creator_email"`
	AppName      string    `bson:"app"`
	Roles        []string  `bson:",omitempty"`
}

var _ auth.AppTokenService = &AppTokenService{}

func appTokensCollection(conn *db.Storage) *dbStorage.Collection {
	c := conn.Collection("app_tokens")
	c.EnsureIndex(mgo.Index{Key: []string{"token"}, Unique: true, Background: true})
	return c
}

func (s *AppTokenService) Insert(t auth.AppToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = appTokensCollection(conn).Insert(appToken(t))
	if mgo.IsDup(err) {
		return auth.ErrAppTokenAlreadyExists
	}
	return err
}

func (s *AppTokenService) FindByToken(token string) (*auth.AppToken, error) {
	results, err := s.findByQuery(bson.M{"token": token})
	if err != nil {
		if err == mgo.ErrNotFound {
			err = auth.ErrAppTokenNotFound
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, auth.ErrAppTokenNotFound
	}
	appToken := auth.AppToken(results[0])
	return &appToken, nil
}

func (s *AppTokenService) FindByAppName(appName string) ([]auth.AppToken, error) {
	return s.findByQuery(bson.M{"app": appName})
}

func (s *AppTokenService) findByQuery(query bson.M) ([]auth.AppToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var tokens []appToken
	err = appTokensCollection(conn).Find(query).All(&tokens)
	if err != nil {
		return nil, err
	}
	authTeams := make([]auth.AppToken, len(tokens))
	for i, t := range tokens {
		authTeams[i] = auth.AppToken(t)
	}
	return authTeams, nil
}

func (s *AppTokenService) Delete(t auth.AppToken) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = appTokensCollection(conn).Remove(bson.M{"token": t.Token})
	if err == mgo.ErrNotFound {
		return auth.ErrAppTokenNotFound
	}
	return err
}
