// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/quota"
)

var _ quota.QuotaStorage = (*quotaStorage)(nil)

type quotaStorage struct {
	collection string
	query      func(string) bson.M
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

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()

	err = conn.Collection(s.collection).Update(
		query,
		bson.M{"$set": bson.M{"quota.limit": limit}},
	)
	span.SetError(err)
	return err
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

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	err = conn.Collection(s.collection).Update(
		query,
		bson.M{"$set": bson.M{"quota.inuse": inUse}},
	)
	return err
}

func (s *quotaStorage) Get(ctx context.Context, name string) (*quota.Quota, error) {
	query := s.query(name)
	span := newMongoDBSpan(ctx, mongoSpanFind, s.collection)
	span.SetQueryStatement(query)
	defer span.Finish()

	var obj quotaObject
	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	defer conn.Close()
	err = conn.Collection(s.collection).Find(query).One(&obj)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, quota.ErrQuotaNotFound
		}
		span.SetError(err)
		return nil, err
	}
	return &obj.Quota, nil
}
