// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/tracker"
)

const trackerCollectionName = "tracker"

type instanceTrackerStorage struct{}

func trackerCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(trackerCollectionName)
}

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

	span := newMongoDBSpan(ctx, mongoSpanUpsertID, trackerCollectionName)
	span.SetMongoID(dbInstance.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	_, err = trackerCollection(conn).UpsertId(dbInstance.Name, dbInstance)
	span.SetError(err)
	return err
}

func (s *instanceTrackerStorage) List(ctx context.Context, maxStale time.Duration) ([]tracker.TrackedInstance, error) {
	query := bson.M{
		"lastupdate": bson.M{"$gt": time.Now().UTC().Add(-maxStale)},
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, trackerCollectionName)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	defer conn.Close()
	var instances []trackedInstance
	err = trackerCollection(conn).Find(query).All(&instances)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	results := make([]tracker.TrackedInstance, len(instances))
	for i := range instances {
		results[i] = tracker.TrackedInstance(instances[i])
	}
	return results, nil
}
