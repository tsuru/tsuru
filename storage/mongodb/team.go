// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/storage"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TeamRepository struct{}

func init() {
	storage.TeamRepository = &TeamRepository{}
}

func teamsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("teams")
}

func (t *TeamRepository) Insert(team storage.Team) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).Insert(team)
	if mgo.IsDup(err) {
		return storage.ErrTeamAlreadyExists
	}
	return err
}

func (t *TeamRepository) FindAll() ([]storage.Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []storage.Team
	err = teamsCollection(conn).Find(nil).All(&teams)
	return teams, err
}

func (t *TeamRepository) FindByName(name string) (*storage.Team, error) {
	var team storage.Team
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = teamsCollection(conn).FindId(name).One(&team)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = storage.ErrTeamNotFound
		}
		return nil, err
	}
	return &team, nil
}

func (t *TeamRepository) FindByNames(names []string) ([]storage.Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []storage.Team
	err = teamsCollection(conn).Find(bson.M{"_id": bson.M{"$in": names}}).All(&teams)
	return teams, err
}

func (t *TeamRepository) Delete(team storage.Team) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).RemoveId(team.Name)
	if err == mgo.ErrNotFound {
		return storage.ErrTeamNotFound
	}
	return err
}
