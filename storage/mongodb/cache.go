// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/cache"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type cacheStorage struct {
	collection string
}

type mongoCacheEntry struct {
	Key      string `bson:"_id"`
	Value    string
	ExpireAt time.Time `bson:",omitempty"`
}

func (s *cacheStorage) cacheCollectionV2() (*mongo.Collection, error) {
	return storagev2.Collection(s.collection)
}

func (s *cacheStorage) GetAll(ctx context.Context, keys ...string) ([]cache.CacheEntry, error) {
	query := mongoBSON.M{"_id": mongoBSON.M{"$in": keys}}

	span := newMongoDBSpan(ctx, mongoSpanFind, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := s.cacheCollectionV2()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	var dbEntries []mongoCacheEntry

	cursor, err := collection.Find(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = cursor.All(ctx, &dbEntries)
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

	collection, err := s.cacheCollectionV2()
	if err != nil {
		span.SetError(err)
		return cache.CacheEntry{}, err
	}
	var dbEntry mongoCacheEntry
	err = collection.FindOne(ctx, mongoBSON.M{"_id": key}).Decode(&dbEntry)
	if err != nil {
		if err == mongo.ErrNoDocuments {
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

	collection, err := s.cacheCollectionV2()
	if err != nil {
		span.SetError(err)
		return err
	}

	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": entry.Key}, mongoBSON.M{"$set": mongoCacheEntry(entry)}, opts)

	if err != nil {
		span.SetError(err)
		return err
	}

	return nil
}
