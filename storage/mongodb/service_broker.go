// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"

	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/service"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type serviceBrokerStorage struct{}

var _ service.ServiceBrokerStorage = &serviceBrokerStorage{}

const serviceBrokerCollectionName = "service_broker"

func serviceBrokerCollection(conn *db.Storage) *dbStorage.Collection {
	coll := conn.Collection(serviceBrokerCollectionName)
	coll.EnsureIndex(mgo.Index{
		Key:    []string{"name"},
		Unique: true,
	})
	return coll
}

func (s *serviceBrokerStorage) Insert(b service.Broker) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = serviceBrokerCollection(conn).Insert(b)
	if err != nil && mgo.IsDup(err) {
		err = service.ErrServiceBrokerAlreadyExists
	}
	return err
}

func (s *serviceBrokerStorage) Update(ctx context.Context, name string, b service.Broker) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = serviceBrokerCollection(conn).Update(bson.M{"name": name}, b)
	if err == mgo.ErrNotFound {
		err = service.ErrServiceBrokerNotFound
	}
	return err
}

func (s *serviceBrokerStorage) Delete(name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = serviceBrokerCollection(conn).Remove(bson.M{"name": name})
	if err == mgo.ErrNotFound {
		err = service.ErrServiceBrokerNotFound
	}
	return err
}

func (s *serviceBrokerStorage) FindAll(ctx context.Context) ([]service.Broker, error) {
	collection, err := storagev2.Collection(serviceBrokerCollectionName)
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
	collection, err := storagev2.Collection(serviceBrokerCollectionName)
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
