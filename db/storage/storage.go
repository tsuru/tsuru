// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"sync"
	"time"

	"github.com/globalsign/mgo"
)

var (
	sessions    = map[string]*mgo.Session{}
	sessionLock sync.RWMutex
	createdMap  = map[string]struct{}{}
	createdLock sync.RWMutex
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

func (c *Collection) Create(info *mgo.CollectionInfo) error {
	createdLock.RLock()
	if _, ok := createdMap[c.Name]; ok {
		createdLock.RUnlock()
		return nil
	}
	createdLock.RUnlock()
	createdLock.Lock()
	defer createdLock.Unlock()
	if _, ok := createdMap[c.Name]; ok {
		return nil
	}
	createdMap[c.Name] = struct{}{}
	return c.Collection.Create(info)
}

func (c *Collection) DropCollection() error {
	createdLock.Lock()
	defer createdLock.Unlock()
	delete(createdMap, c.Name)
	return c.Collection.DropCollection()
}

// Close closes the session with the database.
func (c *Collection) Close() {
	c.Collection.Database.Session.Close()
}

func open(addr string) (*mgo.Session, error) {
	dialInfo, err := mgo.ParseURL(addr)
	if err != nil {
		return nil, err
	}
	dialInfo.FailFast = true
	dialInfo.DialServer = instrumentedDialServer(dialInfo.Timeout)
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	session.SetSyncTimeout(10 * time.Second)
	session.SetSocketTimeout(time.Minute)
	return session, nil
}

// Close closes the storage, releasing the connection.
func (s *Storage) Close() {
	s.session.Close()
}

func (s *Storage) Database(name string) *mgo.Database {
	return s.session.DB(name)
}

// DropDatabase drop database of any given name
func (s *Storage) DropDatabase(name string) error {
	return s.session.DB(name).DropDatabase()
}

// Collection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) Collection(name string) *Collection {
	return &Collection{s.session.DB(s.dbname).C(name)}
}
