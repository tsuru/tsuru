// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/tracker"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type instanceTrackerStorage struct{}

type trackedInstance struct {
	Name       string    `bson:"_id"`
	Port       string    `bson:"port"`
	TLSPort    string    `bson:"tlsport"`
	Addresses  []string  `bson:"addresses"`
	LastUpdate time.Time `bson:"lastupdate"`
}

func (s *instanceTrackerStorage) Notify(ctx context.Context, instance tracker.TrackedInstance) error {
	instance.LastUpdate = time.Now().UTC()
	dbInstance := trackedInstance(instance)

	collection, err := storagev2.TrackerCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsertID, collection.Name())
	span.SetMongoID(dbInstance.Name)
	defer span.Finish()

	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": dbInstance.Name}, dbInstance, options.Replace().SetUpsert(true))
	if err != nil {
		span.SetError(err)
		return err
	}

	return nil
}

func (s *instanceTrackerStorage) List(ctx context.Context, maxStale time.Duration) ([]tracker.TrackedInstance, error) {
	query := mongoBSON.M{
		"lastupdate": mongoBSON.M{"$gt": time.Now().UTC().Add(-maxStale)},
	}

	collection, err := storagev2.TrackerCollection()
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

	var instances []tracker.TrackedInstance
	err = cursor.All(ctx, &instances)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return instances, nil
}
