// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	storagev2 "github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
)

type TeamStorage struct{}

var _ auth.TeamStorage = &TeamStorage{}

type team struct {
	Name         string `bson:"_id"`
	CreatingUser string
	Tags         []string
	Quota        quota.Quota
}

func (s *TeamStorage) Insert(ctx context.Context, t auth.Team) error {
	collection, err := storagev2.TeamsCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanInsert, collection.Name())
	defer span.Finish()

	_, err = collection.InsertOne(ctx, team(t))
	if mongo.IsDuplicateKeyError(err) {
		err = auth.ErrTeamAlreadyExists
	}
	span.SetError(err)
	return err
}

func (s *TeamStorage) Update(ctx context.Context, t auth.Team) error {
	collection, err := storagev2.TeamsCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanUpdateID, collection.Name())
	span.SetMongoID(t.Name)

	defer span.Finish()

	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": t.Name}, t)
	if err == mongo.ErrNoDocuments {
		err = auth.ErrTeamNotFound
	}
	span.SetError(err)
	return err
}

func (s *TeamStorage) FindAll(ctx context.Context) ([]auth.Team, error) {
	return s.findByQuery(ctx, nil)
}

func (s *TeamStorage) FindByName(ctx context.Context, name string) (*auth.Team, error) {
	var t team

	collection, err := storagev2.TeamsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFindID, collection.Name())
	span.SetMongoID(name)
	defer span.Finish()

	err = collection.FindOne(ctx, mongoBSON.M{
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
	query := mongoBSON.M{"_id": mongoBSON.M{"$in": names}}
	return s.findByQuery(ctx, query)
}

func (s *TeamStorage) findByQuery(ctx context.Context, query mongoBSON.M) ([]auth.Team, error) {
	var teams []team
	collection, err := storagev2.TeamsCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()
	cursor, err := collection.Find(ctx, query)
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
	collection, err := storagev2.TeamsCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDeleteID, collection.Name())
	span.SetMongoID(t.Name)
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": t.Name})
	if err == mongo.ErrNoDocuments {
		return auth.ErrTeamNotFound
	}

	if err != nil {
		span.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return auth.ErrTeamNotFound
	}

	return nil
}
