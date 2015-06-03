// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"fmt"
	"sync"

	"gopkg.in/mgo.v2"
)

var (
	session *mgo.Session
	mut     sync.RWMutex
)

// Storage holds the connection with the database.
type Storage struct {
	session *mgo.Session
	dbname  string
}

// Collection represents a database collection. It embeds mgo.Collection for
// operations, and holds a session to MongoDB. The user may close the session
// using the method close.
type Collection struct {
	*mgo.Collection
}

// Close closes the session with the database.
func (c *Collection) Close() {
	c.Collection.Database.Session.Close()
}

func open(addr, dbname string) (*Storage, error) {
	if session == nil {
		var err error
		mut.Lock()
		session, err = mgo.Dial(addr)
		mut.Unlock()
		if err != nil {
			return nil, fmt.Errorf("mongodb: %s", err)
		}
	}
	copy := session.Clone()
	storage := &Storage{session: copy, dbname: dbname}
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
	defer func() {
		if r := recover(); r != nil {
			storage, err = open(addr, dbname)
		}
	}()
	if err = session.Ping(); err != nil {
		mut.Lock()
		session = nil
		mut.Unlock()
	}
	return open(addr, dbname)
}

// Close closes the storage, releasing the connection.
func (s *Storage) Close() {
	s.session.Close()
}

// Collection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) Collection(name string) *Collection {
	return &Collection{s.session.DB(s.dbname).C(name)}
}
