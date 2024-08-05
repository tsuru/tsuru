// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

func (s *teamTokenStorage) Insert(ctx context.Context, t auth.TeamToken) error {
	collection, err := storagev2.TeamTokensCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanInsert, collection.Name())
	defer span.Finish()

	_, err = collection.InsertOne(ctx, teamToken(t))
	if mongo.IsDuplicateKeyError(err) {
		err = auth.ErrTeamTokenAlreadyExists
	}
	span.SetError(err)
	return err
}

func (s *teamTokenStorage) findOne(ctx context.Context, query mongoBSON.M) (*auth.TeamToken, error) {
	results, err := s.findByQuery(ctx, query)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = auth.ErrTeamTokenNotFound
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, auth.ErrTeamTokenNotFound
	}
	return &results[0], nil
}

func (s *teamTokenStorage) FindByToken(ctx context.Context, token string) (*auth.TeamToken, error) {
	return s.findOne(ctx, mongoBSON.M{"token": token})
}

func (s *teamTokenStorage) FindByTokenID(ctx context.Context, tokenID string) (*auth.TeamToken, error) {
	return s.findOne(ctx, mongoBSON.M{"token_id": tokenID})
}

func (s *teamTokenStorage) FindByTeams(ctx context.Context, teamNames []string) ([]auth.TeamToken, error) {
	query := mongoBSON.M{}
	if teamNames != nil {
		query["team"] = mongoBSON.M{"$in": teamNames}
	}
	return s.findByQuery(ctx, query)
}

func (s *teamTokenStorage) findByQuery(ctx context.Context, query mongoBSON.M) ([]auth.TeamToken, error) {

	collection, err := storagev2.TeamTokensCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var tokens []teamToken
	err = cursor.All(ctx, &tokens)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	authTeams := make([]auth.TeamToken, len(tokens))
	for i, t := range tokens {
		authTeams[i] = auth.TeamToken(t)
	}
	return authTeams, nil
}

func (s *teamTokenStorage) UpdateLastAccess(ctx context.Context, token string) error {
	collection, err := storagev2.TeamTokensCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpdate, collection.Name())
	defer span.Finish()

	result, err := collection.UpdateOne(ctx, mongoBSON.M{
		"token": token,
	}, mongoBSON.M{
		"$set": mongoBSON.M{"last_access": time.Now().UTC()},
	})
	if err == mongo.ErrNoDocuments {
		err = auth.ErrTeamTokenNotFound
	}

	if err != nil {
		span.SetError(err)
		return err
	}

	if result.MatchedCount == 0 {
		return auth.ErrTeamTokenNotFound
	}

	return nil
}

func (s *teamTokenStorage) Update(ctx context.Context, token auth.TeamToken) error {
	collection, err := storagev2.TeamTokensCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanUpdate, collection.Name())
	defer span.Finish()

	result, err := collection.ReplaceOne(ctx, mongoBSON.M{"token_id": token.TokenID}, teamToken(token))
	if err == mongo.ErrNoDocuments {
		err = auth.ErrTeamTokenNotFound
	}

	if err != nil {
		span.SetError(err)
		return err
	}

	if result.MatchedCount == 0 {
		return auth.ErrTeamTokenNotFound
	}

	return nil
}

func (s *teamTokenStorage) Delete(ctx context.Context, token string) error {
	collection, err := storagev2.TeamTokensCollection()
	if err != nil {
		return err
	}
	span := newMongoDBSpan(ctx, mongoSpanDelete, collection.Name())
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"token_id": token})
	if err == mongo.ErrNoDocuments {
		err = auth.ErrTeamTokenNotFound
	}
	if err != nil {
		span.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return auth.ErrTeamTokenNotFound
	}
	return nil
}
