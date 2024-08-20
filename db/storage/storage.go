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
	dialInfo.Timeout = 30 * time.Second
	dialInfo.PoolTimeout = time.Minute
	dialInfo.ReadTimeout = time.Minute
	dialInfo.WriteTimeout = time.Minute
	dialInfo.DialServer = instrumentedDialServer(dialInfo.Timeout)
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	session.SetSyncTimeout(10 * time.Second)
	return session, nil
}

// Close closes the storage, releasing the connection.
func (s *Storage) Close() {
	s.session.Close()
}

// Collection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) Collection(name string) *Collection {
	return &Collection{Collection: s.session.DB(s.dbname).C(name)}
}
