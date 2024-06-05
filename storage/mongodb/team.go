// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	storagev2 "github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
)

const teamsCollectionName = "teams"

type TeamStorage struct{}

var _ auth.TeamStorage = &TeamStorage{}

type team struct {
	Name         string `bson:"_id"`
	CreatingUser string
	Tags         []string
	Quota        quota.Quota
}

func teamsCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(teamsCollectionName)
}

func (s *TeamStorage) Insert(ctx context.Context, t auth.Team) error {
	span := newMongoDBSpan(ctx, mongoSpanInsert, teamsCollectionName)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).Insert(team(t))
	if mgo.IsDup(err) {
		err = auth.ErrTeamAlreadyExists
	}
	span.SetError(err)
	return err
}

func (s *TeamStorage) Update(ctx context.Context, t auth.Team) error {
	span := newMongoDBSpan(ctx, mongoSpanUpdateID, teamsCollectionName)
	span.SetMongoID(t.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).UpdateId(t.Name, t)
	if err == mgo.ErrNotFound {
		err = auth.ErrTeamNotFound
	}
	span.SetError(err)
	return err
}

func (s *TeamStorage) FindAll(ctx context.Context) ([]auth.Team, error) {
	return s.findByQuery(ctx, nil)
}

func (s *TeamStorage) FindByName(ctx context.Context, name string) (*auth.Team, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, teamsCollectionName)
	span.SetMongoID(name)
	defer span.Finish()

	var t team

	err := storagev2.Collection(teamsCollectionName).FindOne(ctx, mongoBSON.M{
		"_id": name,
	}).Decode(&t)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = auth.ErrTeamNotFound
		}
		span.SetError(err)
		return nil, err
	}
	team := auth.Team(t)
	return &team, nil
}

func (s *TeamStorage) FindByNames(ctx context.Context, names []string) ([]auth.Team, error) {
	query := bson.M{"_id": bson.M{"$in": names}}
	return s.findByQuery(ctx, query)
}

func (s *TeamStorage) findByQuery(ctx context.Context, query bson.M) ([]auth.Team, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, teamsCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	var teams []team
	cursor, err := storagev2.Collection(teamsCollectionName).Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	cursor.All(ctx, &teams)
	authTeams := make([]auth.Team, len(teams))
	for i, t := range teams {
		authTeams[i] = auth.Team(t)
	}
	return authTeams, nil
}

func (s *TeamStorage) Delete(ctx context.Context, t auth.Team) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, teamsCollectionName)
	span.SetMongoID(t.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = teamsCollection(conn).RemoveId(t.Name)
	if err == mgo.ErrNotFound {
		err = auth.ErrTeamNotFound
	}
	span.SetError(err)
	return err
}
