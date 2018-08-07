// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"time"

	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/app"
)

type cacheStorage struct {
	collection string
}

type mongoCacheEntry struct {
	Key      string `bson:"_id"`
	Value    string
	ExpireAt time.Time `bson:",omitempty"`
}

func (s *cacheStorage) cacheCollection(conn *db.Storage) *dbStorage.Collection {
	c := conn.Collection(s.collection)
	// Ideally ExpireAfter would be 0, but due to mgo bug this needs to be at
	// least a second.
	c.EnsureIndex(mgo.Index{Key: []string{"expireat"}, ExpireAfter: time.Second})
	return c
}

func (s *cacheStorage) GetAll(keys ...string) ([]app.CacheEntry, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var dbEntries []mongoCacheEntry
	err = s.cacheCollection(conn).Find(bson.M{"_id": bson.M{"$in": keys}}).All(&dbEntries)
	if err != nil {
		return nil, err
	}
	entries := make([]app.CacheEntry, len(dbEntries))
	for i := range dbEntries {
		entries[i] = app.CacheEntry(dbEntries[i])
	}
	return entries, nil
}

func (s *cacheStorage) Get(key string) (app.CacheEntry, error) {
	conn, err := db.Conn()
	if err != nil {
		return app.CacheEntry{}, err
	}
	defer conn.Close()
	var dbEntry mongoCacheEntry
	err = s.cacheCollection(conn).FindId(key).One(&dbEntry)
	if err != nil {
		if err == mgo.ErrNotFound {
			return app.CacheEntry{}, app.ErrEntryNotFound
		}
		return app.CacheEntry{}, err
	}
	return app.CacheEntry(dbEntry), nil
}

func (s *cacheStorage) Put(entry app.CacheEntry) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = s.cacheCollection(conn).UpsertId(entry.Key, mongoCacheEntry(entry))
	return err
}
