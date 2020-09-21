// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/cache"
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

func (s *cacheStorage) GetAll(ctx context.Context, keys ...string) ([]cache.CacheEntry, error) {
	query := bson.M{"_id": bson.M{"$in": keys}}

	span := newMongoDBSpan(ctx, mongoSpanFind, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	var dbEntries []mongoCacheEntry
	err = s.cacheCollection(conn).Find(query).All(&dbEntries)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	entries := make([]cache.CacheEntry, len(dbEntries))
	for i := range dbEntries {
		entries[i] = cache.CacheEntry(dbEntries[i])
	}
	return entries, nil
}

func (s *cacheStorage) Get(ctx context.Context, key string) (cache.CacheEntry, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, s.collection)
	span.SetMongoID(key)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return cache.CacheEntry{}, err
	}
	defer conn.Close()
	var dbEntry mongoCacheEntry
	err = s.cacheCollection(conn).FindId(key).One(&dbEntry)
	if err != nil {
		if err == mgo.ErrNotFound {
			return cache.CacheEntry{}, cache.ErrEntryNotFound
		}
		span.SetError(err)
		return cache.CacheEntry{}, err
	}
	return cache.CacheEntry(dbEntry), nil
}

func (s *cacheStorage) Put(ctx context.Context, entry cache.CacheEntry) error {
	span := newMongoDBSpan(ctx, mongoSpanUpsertID, s.collection)
	span.SetMongoID(entry.Key)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	_, err = s.cacheCollection(conn).UpsertId(entry.Key, mongoCacheEntry(entry))
	span.SetError(err)
	return err
}
