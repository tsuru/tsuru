// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/router"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type dynamicRouter struct {
	Name           string `bson:"_id"`
	Type           string
	ReadinessGates []string               `bson:",omitempty"`
	Config         map[string]interface{} `bson:",omitempty"`
}

type dynamicRouterStorage struct{}

func (s *dynamicRouterStorage) Save(ctx context.Context, dr router.DynamicRouter) error {
	collection, err := storagev2.DynamicRoutersCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanUpsertID, collection.Name())
	span.SetMongoID(dr.Name)
	defer span.Finish()

	_, err = collection.ReplaceOne(ctx, mongoBSON.M{"_id": dr.Name}, dynamicRouter(dr), options.Replace().SetUpsert(true))
	if err != nil {
		span.SetError(err)
		return err
	}
	return nil
}

func (s *dynamicRouterStorage) Get(ctx context.Context, name string) (*router.DynamicRouter, error) {

	collection, err := storagev2.DynamicRoutersCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFindID, collection.Name())
	span.SetMongoID(name)
	defer span.Finish()

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
	collection, err := storagev2.DynamicRoutersCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	defer span.Finish()

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

	collection, err := storagev2.DynamicRoutersCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDeleteID, collection.Name())
	span.SetMongoID(name)
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": name})
	if err != nil {
		span.SetError(err)
		if err == mongo.ErrNoDocuments {
			return router.ErrDynamicRouterNotFound
		}
		return err
	}

	if result.DeletedCount == 0 {
		return router.ErrDynamicRouterNotFound
	}

	return nil
}
