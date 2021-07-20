// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	mgo "github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/types/service"
)

type serviceBrokerStorage struct{}

var _ service.ServiceBrokerStorage = (*serviceBrokerStorage)(nil)

func serviceBrokerCollection(conn *db.Storage) *dbStorage.Collection {
	coll := conn.Collection("service_broker")
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

func (s *serviceBrokerStorage) Update(name string, b service.Broker) error {
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

func (s *serviceBrokerStorage) FindAll() ([]service.Broker, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var brokers []service.Broker
	err = serviceBrokerCollection(conn).Find(nil).All(&brokers)
	return brokers, err
}

func (s *serviceBrokerStorage) Find(name string) (service.Broker, error) {
	conn, err := db.Conn()
	if err != nil {
		return service.Broker{}, err
	}
	defer conn.Close()
	var b service.Broker
	err = serviceBrokerCollection(conn).Find(bson.M{"name": name}).One(&b)
	if err == mgo.ErrNotFound {
		err = service.ErrServiceBrokerNotFound
	}
	return b, err
}
