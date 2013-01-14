// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package db encapsulates tsuru connection with MongoDB.
//
// The function Open dials to MongoDB and returns a connection (represented by
// the Storage type). It manages an internal pool of connections, and
// reconnects in case of failures. That means that you should not store
// references to the connection, but always call Open.
package db

import (
	"fmt"
	"github.com/globocom/config"
	"labix.org/v2/mgo"
	"sync"
)

// pool of connections
var conn = make(map[string]*Storage)

// Storage holds the connection with the database.
type Storage struct {
	collections map[string]*mgo.Collection
	session     *mgo.Session
	dbname      string
	mut         sync.RWMutex
}

func open(addr, dbname string) (*Storage, error) {
	key := addr + dbname
	session, err := mgo.Dial(addr)
	if err != nil {
		return nil, err
	}
	storage := &Storage{
		session:     session,
		collections: make(map[string]*mgo.Collection),
		dbname:      dbname,
	}
	conn[key] = storage
	return storage, nil
}

// Open dials to the MongoDB database, and return the connection (represented
// by the type Storage).
//
// addr is a MongoDB connection URI, and dbname is the name of the database.
//
// This function returns a pointer to a Storage, or a non-nil error in case of
// any failure.
func Open(addr, dbname string) (storage *Storage, err error) {
	var ok bool
	defer func() {
		if r := recover(); r != nil {
			storage, err = open(addr, dbname)
		}
	}()
	key := addr + dbname
	if storage, ok = conn[key]; ok {
		storage.session.Ping()
		return storage, nil
	}
	return open(addr, dbname)
}

// Conn reads the tsuru config and calls Open to get a database connection.
//
// Most tsuru packages should probably use this function. Open is intended for
// use when supporting more than one database.
func Conn() (storage *Storage, err error) {
	url, err := config.GetString("database:url")
	if err != nil {
		return nil, fmt.Errorf("configuration error: %s", err)
	}
	dbname, err := config.GetString("database:name")
	if err != nil {
		return nil, fmt.Errorf("configuration error: %s", err)
	}
	return Open(url, dbname)
}

// Close closes the connection.
//
// You can take advantage of defer statement, and write code that look like this:
//
//     st, err := Open("localhost:27017", "tsuru")
//     if err != nil {
//         panic(err)
//     }
//     defer st.Close()
func (s *Storage) Close() {
	s.session.Close()
}

// Collection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) Collection(name string) *mgo.Collection {
	s.mut.RLock()
	collection, ok := s.collections[name]
	s.mut.RUnlock()

	if !ok {
		collection = s.session.DB(s.dbname).C(name)
		s.mut.Lock()
		s.collections[name] = collection
		s.mut.Unlock()
	}
	return collection
}

// Apps returns the apps collection from MongoDB.
func (s *Storage) Apps() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.Collection("apps")
	c.EnsureIndex(nameIndex)
	return c
}

// Services returns the services collection from MongoDB.
func (s *Storage) Services() *mgo.Collection {
	c := s.Collection("services")
	return c
}

// ServiceInstances returns the services_instances collection from MongoDB.
func (s *Storage) ServiceInstances() *mgo.Collection {
	return s.Collection("service_instances")
}

// Users returns the users collection from MongoDB.
func (s *Storage) Users() *mgo.Collection {
	emailIndex := mgo.Index{Key: []string{"email"}, Unique: true}
	c := s.Collection("users")
	c.EnsureIndex(emailIndex)
	return c
}

// Teams returns the teams collection from MongoDB.
func (s *Storage) Teams() *mgo.Collection {
	return s.Collection("teams")
}
