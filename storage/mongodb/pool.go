// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/types/provision"
)

var _ provision.PoolStorage = &PoolStorage{}

type PoolStorage struct{}

func (ps *PoolStorage) FindAll(ctx context.Context) ([]provision.Pool, error) {
	return findPoolsByQuery(ctx, nil)
}

func (ps *PoolStorage) FindByName(ctx context.Context, name string) (*provision.Pool, error) {
	return findPoolByQuery(ctx, bson.M{"_id": name})
}

func findPoolByQuery(ctx context.Context, filter bson.M) (*provision.Pool, error) {
	pools, err := findPoolsByQuery(ctx, filter)
	if err != nil {
		if err == mgo.ErrNotFound {
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

func findPoolsByQuery(ctx context.Context, filter bson.M) ([]provision.Pool, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	pools := []provision.Pool{}
	err = conn.Pools().Find(filter).All(&pools)
	if err != nil {
		return nil, err
	}
	return pools, err
}
