// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TeamStorage struct{}

var _ storage.TeamStorage = &TeamStorage{}

type team struct {
	Name         string `bson:"_id"`
	CreatingUser string
}

func teamsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("teams")
}

func (s *TeamStorage) Insert(t types.Team) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).Insert(team(t))
	if mgo.IsDup(err) {
		return types.ErrTeamAlreadyExists
	}
	return err
}

func (s *TeamStorage) FindAll() ([]types.Team, error) {
	return s.findByQuery(nil)
}

func (s *TeamStorage) FindByName(name string) (*types.Team, error) {
	var t team
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = teamsCollection(conn).FindId(name).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = types.ErrTeamNotFound
		}
		return nil, err
	}
	team := types.Team(t)
	return &team, nil
}

func (s *TeamStorage) FindByNames(names []string) ([]types.Team, error) {
	query := bson.M{"_id": bson.M{"$in": names}}
	return s.findByQuery(query)
}

func (s *TeamStorage) findByQuery(query bson.M) ([]types.Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []team
	err = teamsCollection(conn).Find(query).All(&teams)
	if err != nil {
		return nil, err
	}
	authTeams := make([]types.Team, len(teams))
	for i, t := range teams {
		authTeams[i] = types.Team(t)
	}
	return authTeams, nil
}

func (s *TeamStorage) Delete(t types.Team) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).RemoveId(t.Name)
	if err == mgo.ErrNotFound {
		return types.ErrTeamNotFound
	}
	return err
}
