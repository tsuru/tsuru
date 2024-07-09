// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var _ app.PlatformStorage = &PlatformStorage{}

const platformsCollectionName = "platforms"

type PlatformStorage struct{}

type platform struct {
	Name     string `bson:"_id"`
	Disabled bool   `bson:",omitempty"`
}

func (s *PlatformStorage) Insert(ctx context.Context, p app.Platform) error {

	span := newMongoDBSpan(ctx, mongoSpanInsert, platformsCollectionName)
	defer span.Finish()

	collection, err := storagev2.Collection(platformsCollectionName)
	if err != nil {
		span.SetError(err)
		return err
	}

	_, err = collection.InsertOne(ctx, platform(p))
	if err != nil {
		span.SetError(err)
		if mongo.IsDuplicateKeyError(err) {
			return app.ErrDuplicatePlatform
		}
		return err
	}

	return nil
}

func (s *PlatformStorage) FindByName(ctx context.Context, name string) (*app.Platform, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, platformsCollectionName)
	defer span.Finish()

	var p platform
	collection, err := storagev2.Collection(platformsCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&p)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			err = app.ErrPlatformNotFound
		}
		span.SetError(err)
		return nil, err
	}
	platform := app.Platform(p)
	return &platform, nil
}

func (s *PlatformStorage) FindAll(ctx context.Context) ([]app.Platform, error) {
	return s.findByQuery(ctx, mongoBSON.M{})
}

func (s *PlatformStorage) FindEnabled(ctx context.Context) ([]app.Platform, error) {
	query := mongoBSON.M{"$or": []mongoBSON.M{{"disabled": false}, {"disabled": mongoBSON.M{"$exists": false}}}}
	return s.findByQuery(ctx, query)
}

func (s *PlatformStorage) findByQuery(ctx context.Context, query mongoBSON.M) ([]app.Platform, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, platformsCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(platformsCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var platforms []platform
	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = cursor.All(ctx, &platforms)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	appPlatforms := make([]app.Platform, len(platforms))
	for i, p := range platforms {
		appPlatforms[i] = app.Platform(p)
	}
	return appPlatforms, nil
}

func (s *PlatformStorage) Update(ctx context.Context, p app.Platform) error {
	span := newMongoDBSpan(ctx, mongoSpanUpdate, platformsCollectionName)
	span.SetMongoID(p.Name)
	defer span.Finish()

	collection, err := storagev2.Collection(platformsCollectionName)
	if err != nil {
		span.SetError(err)
		return err
	}
	result, err := collection.UpdateOne(ctx, mongoBSON.M{"_id": p.Name}, mongoBSON.M{"$set": mongoBSON.M{"disabled": p.Disabled}})

	if err != nil {
		span.SetError(err)
		return err
	}

	if result.ModifiedCount == 0 {
		return app.ErrPlatformNotFound
	}
	return nil
}

func (s *PlatformStorage) Delete(ctx context.Context, p app.Platform) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, platformsCollectionName)
	span.SetMongoID(p.Name)
	defer span.Finish()

	collection, err := storagev2.Collection(platformsCollectionName)
	if err != nil {
		span.SetError(err)
		return err
	}

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": p.Name})

	if err == mongo.ErrNoDocuments {
		span.SetError(err)
		return app.ErrPlatformNotFound
	}

	if err != nil {
		span.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return app.ErrPlatformNotFound
	}

	return nil
}
