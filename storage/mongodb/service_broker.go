// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/service"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type serviceBrokerStorage struct{}

var _ service.ServiceBrokerStorage = &serviceBrokerStorage{}

func (s *serviceBrokerStorage) Insert(ctx context.Context, b service.Broker) error {
	collection, err := storagev2.ServiceBrokerCollection()
	if err != nil {
		return err
	}
	_, err = collection.InsertOne(ctx, b)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		err = service.ErrServiceBrokerAlreadyExists
	}
	return err
}

func (s *serviceBrokerStorage) Update(ctx context.Context, name string, b service.Broker) error {
	collection, err := storagev2.ServiceBrokerCollection()
	if err != nil {
		return err
	}
	result, err := collection.ReplaceOne(ctx, mongoBSON.M{"name": name}, b)
	if err == mongo.ErrNoDocuments {
		err = service.ErrServiceBrokerNotFound
	}
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		err = service.ErrServiceBrokerNotFound
	}

	return err
}

func (s *serviceBrokerStorage) Delete(ctx context.Context, name string) error {
	collection, err := storagev2.ServiceBrokerCollection()
	if err != nil {
		return err
	}
	result, err := collection.DeleteOne(ctx, mongoBSON.M{"name": name})
	if err == mongo.ErrNoDocuments {
		return service.ErrServiceBrokerNotFound
	}
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return service.ErrServiceBrokerNotFound
	}

	return nil
}

func (s *serviceBrokerStorage) FindAll(ctx context.Context) ([]service.Broker, error) {
	collection, err := storagev2.ServiceBrokerCollection()
	if err != nil {
		return nil, err
	}
	var brokers []service.Broker
	cursor, err := collection.Find(ctx, mongoBSON.M{})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &brokers)
	if err != nil {
		return nil, err
	}
	return brokers, nil
}

func (s *serviceBrokerStorage) Find(ctx context.Context, name string) (service.Broker, error) {
	collection, err := storagev2.ServiceBrokerCollection()
	if err != nil {
		return service.Broker{}, err
	}

	var b service.Broker
	err = collection.FindOne(ctx, mongoBSON.M{"name": name}).Decode(&b)
	if err == mongo.ErrNoDocuments {
		err = service.ErrServiceBrokerNotFound
	}
	if err != nil {
		return service.Broker{}, err
	}
	return b, nil
}
