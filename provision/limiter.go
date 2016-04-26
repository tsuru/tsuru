// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"sync"
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var _ ActionLimiter = &LocalLimiter{}
var noop = func() {}

type ActionLimiter interface {
	SetLimit(uint)
	Start(action string) func()
	Len(action string) int
}

type LocalLimiter struct {
	sync.Mutex
	chMap map[string]chan struct{}
	limit uint
}

func (l *LocalLimiter) SetLimit(i uint) {
	l.limit = i
	l.chMap = nil
	if i != 0 {
		l.chMap = make(map[string]chan struct{})
	}
}

func (l *LocalLimiter) actionEntry(action string) chan struct{} {
	l.Lock()
	if l.chMap == nil {
		l.Unlock()
		return nil
	}
	if l.chMap[action] == nil {
		l.chMap[action] = make(chan struct{}, l.limit)
	}
	limitChan := l.chMap[action]
	l.Unlock()
	return limitChan
}

func (l *LocalLimiter) Start(action string) func() {
	ch := l.actionEntry(action)
	if ch == nil {
		return noop
	}
	ch <- struct{}{}
	return func() {
		<-ch
	}
}

func (l *LocalLimiter) Len(action string) int {
	return len(l.actionEntry(action))
}

type MongodbLimiter struct {
	limit uint
}

func (l *MongodbLimiter) SetLimit(i uint) {
	l.limit = i
}

func (l *MongodbLimiter) collection() *storage.Collection {
	if l.limit == 0 {
		return nil
	}
	conn, err := db.Conn()
	if err != nil {
		return nil
	}
	return conn.Limiter()
}

func (l *MongodbLimiter) Start(action string) func() {
	coll := l.collection()
	if coll == nil {
		return noop
	}
	defer coll.Close()
	for {
		_, err := coll.Upsert(bson.M{"_id": action, "count": bson.M{"$lt": l.limit}}, bson.M{"$inc": bson.M{"count": 1}})
		if err == nil {
			break
		}
		if !mgo.IsDup(err) {
			return noop
		}
		time.Sleep(100 * time.Millisecond)
	}
	return func() {
		doneColl := l.collection()
		if doneColl == nil {
			return
		}
		defer doneColl.Close()
		doneColl.Update(bson.M{"_id": action, "count": bson.M{"$gt": 0}}, bson.M{"$inc": bson.M{"count": -1}})
	}
}

func (l *MongodbLimiter) Len(action string) int {
	coll := l.collection()
	if coll == nil {
		return 0
	}
	var result struct {
		Count int
	}
	coll.Find(bson.M{"_id": action}).One(&result)
	return result.Count
}
