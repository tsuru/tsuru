// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"runtime"
	"sync"
	"time"

	"gopkg.in/mgo.v2"
)

var (
	sessions    = map[string]*mgo.Session{}
	sessionLock sync.RWMutex
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

func open(addr string) (*mgo.Session, error) {
	dialInfo, err := mgo.ParseURL(addr)
	if err != nil {
		return nil, err
	}
	dialInfo.FailFast = true
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	session.SetSyncTimeout(10 * time.Second)
	session.SetSocketTimeout(1 * time.Minute)
	return session, nil
}

// Open dials to the MongoDB database, and return the connection (represented
// by the type Storage).
//
// addr is a MongoDB connection URI, and dbname is the name of the database.
//
// This function returns a pointer to a Storage, or a non-nil error in case of
// any failure.
func Open(addr, dbname string) (storage *Storage, err error) {
	sessionLock.RLock()
	if sessions[addr] == nil {
		sessionLock.RUnlock()
		sessionLock.Lock()
		if sessions[addr] == nil {
			sessions[addr], err = open(addr)
		}
		sessionLock.Unlock()
		if err != nil {
			return
		}
	} else {
		sessionLock.RUnlock()
	}
	cloned := sessions[addr].Clone()
	runtime.SetFinalizer(cloned, sessionFinalizer)
	storage = &Storage{
		session: cloned,
		dbname:  dbname,
	}
	return
}

func sessionFinalizer(session *mgo.Session) {
	session.Close()
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
