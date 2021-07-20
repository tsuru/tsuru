// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"sync"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

var _ ActionLimiter = (*LocalLimiter)(nil)
var noop = func() {}

type ActionLimiter interface {
	Initialize(uint)
	Start(action string) func()
	Len(action string) int
}

type LocalLimiter struct {
	sync.Mutex
	chMap map[string]chan struct{}
	limit uint
}

func (l *LocalLimiter) Initialize(i uint) {
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
	limit          uint
	idsCh          chan bson.ObjectId
	quitCh         chan struct{}
	updateInterval time.Duration
	maxStale       time.Duration
}

func (l *MongodbLimiter) Initialize(i uint) {
	l.limit = i
	if l.limit == 0 {
		return
	}
	l.idsCh = make(chan bson.ObjectId, 10)
	if l.updateInterval == 0 {
		l.updateInterval = 10 * time.Second
	}
	if l.maxStale == 0 {
		l.maxStale = 30 * time.Second
	}
	l.quitCh = make(chan struct{})
	go l.timeUpdater()
}

func (l *MongodbLimiter) stop() {
	if l.quitCh != nil {
		l.quitCh <- struct{}{}
	}
}

func (l *MongodbLimiter) timeUpdater() {
	var ids []bson.ObjectId
	var timeoutCh <-chan time.Time
	for {
		select {
		case id := <-l.idsCh:
			ids = append(ids, id)
			timeoutCh = time.After(l.updateInterval)
			continue
		case <-timeoutCh:
		case <-l.quitCh:
			return
		}
		if len(ids) == 0 {
			continue
		}
		timeoutCh = time.After(l.updateInterval)
		coll := l.collection()
		if coll == nil {
			continue
		}
		for i := 0; i < len(ids); i++ {
			err := coll.Update(bson.M{
				"elements.id": ids[i],
			}, bson.M{
				"$set": bson.M{"elements.$.update": time.Now().UTC()},
			})
			if err == mgo.ErrNotFound {
				ids = append(ids[:i], ids[i+1:]...)
				i--
			}
		}
		coll.Close()
	}
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
	var pushedId bson.ObjectId
	for {
		coll.RemoveAll(bson.M{"elements.update": bson.M{"$lt": time.Now().Add(-l.maxStale).UTC()}})
		pushedId = bson.NewObjectId()
		_, err := coll.Upsert(bson.M{
			"_id":                                 action,
			fmt.Sprintf("elements.%d", l.limit-1): bson.M{"$exists": false},
		}, bson.M{
			"$push": bson.M{"elements": bson.M{"id": pushedId, "update": time.Now().UTC()}},
		})
		if err == nil {
			l.idsCh <- pushedId
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
		doneColl.Update(bson.M{"_id": action}, bson.M{"$pull": bson.M{"elements": bson.M{"id": pushedId}}})
	}
}

func (l *MongodbLimiter) Len(action string) int {
	coll := l.collection()
	if coll == nil {
		return 0
	}
	defer coll.Close()
	var result struct {
		Elements []interface{}
	}
	coll.Find(bson.M{"_id": action}).One(&result)
	return len(result.Elements)
}
