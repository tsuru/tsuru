// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var _ provision.PoolStorage = &PoolStorage{}

type PoolStorage struct{}

func (ps *PoolStorage) FindAll(ctx context.Context) ([]provision.Pool, error) {
	return findPoolsByQuery(ctx, mongoBSON.M{})
}

func (ps *PoolStorage) FindByName(ctx context.Context, name string) (*provision.Pool, error) {
	return findPoolByQuery(ctx, mongoBSON.M{"_id": name})
}

func findPoolByQuery(ctx context.Context, filter mongoBSON.M) (*provision.Pool, error) {
	pools, err := findPoolsByQuery(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, provision.ErrPoolNotFound
		}
		return nil, err
	}
	switch len(pools) {
	case 0:
		return nil, provision.ErrPoolNotFound
	case 1:
		return &pools[0], nil
	default:
		return nil, provision.ErrTooManyPoolsFound
	}
}

func findPoolsByQuery(ctx context.Context, filter mongoBSON.M) ([]provision.Pool, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, "pool")
	span.SetQueryStatement(filter)
	defer span.Finish()

	collection, err := storagev2.PoolCollection()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	pools := []provision.Pool{}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = cursor.All(ctx, &pools)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return pools, err
}
