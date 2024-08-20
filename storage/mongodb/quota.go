// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var _ quota.QuotaStorage = &quotaStorage{}

type quotaStorage struct {
	collection string
	query      func(string) mongoBSON.M
}

type quotaObject struct {
	Quota quota.Quota
}

func (s *quotaStorage) SetLimit(ctx context.Context, name string, limit int) error {
	_, err := s.Get(ctx, name)
	if err != nil {
		return err
	}

	query := s.query(name)
	span := newMongoDBSpan(ctx, mongoSpanUpdate, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(s.collection)
	if err != nil {
		span.SetError(err)
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		query,
		mongoBSON.M{"$set": mongoBSON.M{"quota.limit": limit}},
	)

	if err != nil {
		span.SetError(err)
		return err
	}

	return nil
}

func (s *quotaStorage) Set(ctx context.Context, name string, inUse int) error {
	_, err := s.Get(ctx, name)
	if err != nil {
		return err
	}

	query := s.query(name)
	span := newMongoDBSpan(ctx, mongoSpanUpdate, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(s.collection)
	if err != nil {
		span.SetError(err)
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		query,
		mongoBSON.M{"$set": mongoBSON.M{"quota.inuse": inUse}},
	)

	if err != nil {
		span.SetError(err)
		return err
	}

	return nil
}

func (s *quotaStorage) Get(ctx context.Context, name string) (*quota.Quota, error) {
	query := s.query(name)
	span := newMongoDBSpan(ctx, mongoSpanFind, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	collection, err := storagev2.Collection(s.collection)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	var obj quotaObject
	err = collection.FindOne(ctx, query).Decode(&obj)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, quota.ErrQuotaNotFound
		}
		span.SetError(err)
		return nil, err
	}
	return &obj.Quota, nil
}
