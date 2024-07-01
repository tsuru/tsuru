// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/globalsign/mgo"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/router"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const dynamicRouterCollectionName = "dynamic_routers"

type dynamicRouter struct {
	Name           string `bson:"_id"`
	Type           string
	ReadinessGates []string               `bson:",omitempty"`
	Config         map[string]interface{} `bson:",omitempty"`
}

type dynamicRouterStorage struct{}

func (s *dynamicRouterStorage) coll(conn *db.Storage) *dbStorage.Collection {
	return conn.Collection(dynamicRouterCollectionName)
}

func (s *dynamicRouterStorage) Save(ctx context.Context, dr router.DynamicRouter) error {
	span := newMongoDBSpan(ctx, mongoSpanUpsertID, dynamicRouterCollectionName)
	span.SetMongoID(dr.Name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	_, err = s.coll(conn).UpsertId(dr.Name, dynamicRouter(dr))
	if err != nil {
		span.SetError(err)
		return err
	}
	return nil
}

func (s *dynamicRouterStorage) Get(ctx context.Context, name string) (*router.DynamicRouter, error) {
	span := newMongoDBSpan(ctx, mongoSpanFindID, dynamicRouterCollectionName)
	span.SetMongoID(name)
	defer span.Finish()

	collection, err := storagev2.Collection(dynamicRouterCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	var dr dynamicRouter
	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&dr)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, router.ErrDynamicRouterNotFound
		}
		span.SetError(err)
		return nil, err
	}
	result := router.DynamicRouter(dr)
	return &result, nil
}

func (s *dynamicRouterStorage) List(ctx context.Context) ([]router.DynamicRouter, error) {
	span := newMongoDBSpan(ctx, mongoSpanFind, dynamicRouterCollectionName)
	defer span.Finish()

	collection, err := storagev2.Collection(dynamicRouterCollectionName)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	var drs []dynamicRouter
	cursor, err := collection.Find(ctx, mongoBSON.M{})
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	err = cursor.All(ctx, &drs)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	result := make([]router.DynamicRouter, len(drs))
	for i := range drs {
		result[i] = router.DynamicRouter(drs[i])
	}
	return result, nil
}

func (s *dynamicRouterStorage) Remove(ctx context.Context, name string) error {
	span := newMongoDBSpan(ctx, mongoSpanDeleteID, dynamicRouterCollectionName)
	span.SetMongoID(name)
	defer span.Finish()

	conn, err := db.Conn()
	if err != nil {
		span.SetError(err)
		return err
	}
	defer conn.Close()
	err = s.coll(conn).RemoveId(name)
	if err != nil {
		span.SetError(err)
		if err == mgo.ErrNotFound {
			return router.ErrDynamicRouterNotFound
		}
		return err
	}
	return nil
}
