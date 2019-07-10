// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/tracker"
)

type instanceTrackerStorage struct{}

func trackerCollection(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection("tracker")
}

type trackedInstance struct {
	Name       string    `bson:"_id"`
	Port       string    `bson:"port"`
	TLSPort    string    `bson:"tlsport"`
	Addresses  []string  `bson:"addresses"`
	LastUpdate time.Time `bson:"lastupdate"`
}

func (s *instanceTrackerStorage) Notify(instance tracker.TrackedInstance) error {
	instance.LastUpdate = time.Now().UTC()
	dbInstance := trackedInstance(instance)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = trackerCollection(conn).UpsertId(dbInstance.Name, dbInstance)
	return err
}

func (s *instanceTrackerStorage) List(maxStale time.Duration) ([]tracker.TrackedInstance, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var instances []trackedInstance
	err = trackerCollection(conn).Find(bson.M{
		"lastupdate": bson.M{"$gt": time.Now().UTC().Add(-maxStale)},
	}).All(&instances)
	if err != nil {
		return nil, err
	}
	results := make([]tracker.TrackedInstance, len(instances))
	for i := range instances {
		results[i] = tracker.TrackedInstance(instances[i])
	}
	return results, nil
}
