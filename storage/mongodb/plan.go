// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ app.PlanStorage = &PlanStorage{}

type PlanStorage struct{}

type planOnMongoDB struct {
	Name     string `bson:"_id"`
	Memory   int64
	CPUMilli int
	CPUBurst *app.CPUBurst
	Default  bool
	Override *app.PlanOverride `bson:"-"`
}

func (s *PlanStorage) Insert(ctx context.Context, p app.Plan) error {
	collection, err := storagev2.PlansCollection()
	if err != nil {
		return err
	}

	if p.Default {
		query := mongoBSON.M{"default": true}
		span := newMongoDBSpan(ctx, mongoSpanUpdateAll, collection.Name())
		span.SetQueryStatement(query)
		defer span.Finish()

		_, err = collection.UpdateMany(ctx, query, mongoBSON.M{"$unset": mongoBSON.M{"default": false}})
		if err != nil {
			span.SetError(err)
			return err
		}
	}

	span := newMongoDBSpan(ctx, mongoSpanInsert, collection.Name())
	defer span.Finish()

	_, err = collection.InsertOne(ctx, planOnMongoDB(p))
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return app.ErrPlanAlreadyExists
	}
	span.SetError(err)
	return err
}

func (s *PlanStorage) FindAll(ctx context.Context) ([]app.Plan, error) {
	return s.findByQuery(ctx, mongoBSON.M{})
}

func (s *PlanStorage) FindDefault(ctx context.Context) (*app.Plan, error) {
	plans, err := s.findByQuery(ctx, mongoBSON.M{"default": true})
	if err != nil {
		return nil, err
	}
	if len(plans) > 1 {
		return nil, app.ErrPlanDefaultAmbiguous
	}
	if len(plans) == 0 {
		return nil, app.ErrPlanDefaultNotFound
	}
	return &plans[0], nil
}

func (s *PlanStorage) findByQuery(ctx context.Context, query mongoBSON.M) ([]app.Plan, error) {
	collection, err := storagev2.PlansCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetQueryStatement(query)
	defer span.Finish()

	cursor, err := collection.Find(ctx, query, &options.FindOptions{Sort: mongoBSON.M{"_id": 1}})
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	plans := []planOnMongoDB{}
	err = cursor.All(ctx, &plans)
	if err != nil {
		span.SetError(err)
		return nil, err
	}

	appPlans := make([]app.Plan, len(plans))
	for i, p := range plans {
		appPlans[i] = app.Plan(p)
	}
	return appPlans, nil
}

func (s *PlanStorage) FindByName(ctx context.Context, name string) (*app.Plan, error) {
	var p planOnMongoDB
	collection, err := storagev2.PlansCollection()
	if err != nil {
		return nil, err
	}

	span := newMongoDBSpan(ctx, mongoSpanFind, collection.Name())
	span.SetMongoID(name)
	defer span.Finish()

	err = collection.FindOne(ctx, mongoBSON.M{"_id": name}).Decode(&p)
	if err != nil {
		span.SetError(err)
		if err == mongo.ErrNoDocuments {
			err = app.ErrPlanNotFound
		}
		return nil, err
	}
	plan := app.Plan(p)
	return &plan, nil
}

func (s *PlanStorage) Delete(ctx context.Context, p app.Plan) error {
	collection, err := storagev2.PlansCollection()
	if err != nil {
		return err
	}

	span := newMongoDBSpan(ctx, mongoSpanDelete, collection.Name())
	span.SetMongoID(p.Name)
	defer span.Finish()

	result, err := collection.DeleteOne(ctx, mongoBSON.M{"_id": p.Name})
	if err == mongo.ErrNoDocuments {
		return app.ErrPlanNotFound
	}
	if err != nil {
		span.SetError(err)
		return err
	}

	if result.DeletedCount == 0 {
		return app.ErrPlanNotFound
	}

	return nil
}
