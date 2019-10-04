// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"sync"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
)

var (
	lockUpdateInterval   = 30 * time.Second
	lockExpireTimeout    = 5 * time.Minute
	eventCleanerInterval = 5 * time.Minute
	updater              = lockUpdater{
		once: &sync.Once{},
	}
	cleaner = eventCleaner{
		once: &sync.Once{},
	}
)

type eventCleaner struct {
	once   *sync.Once
	stopCh chan struct{}
}

func (l *eventCleaner) start() {
	l.once.Do(func() {
		l.stopCh = make(chan struct{})
		go l.spin()
	})
}

func (l *eventCleaner) stop() {
	if l.stopCh == nil {
		return
	}
	l.stopCh <- struct{}{}
	l.stopCh = nil
	l.once = &sync.Once{}
}

func (l *eventCleaner) tryCleaning() error {
	conn, err := db.Conn()
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error getting db conn")
	}
	now := time.Now().UTC()
	coll := conn.Events()
	var allData []eventData
	err = coll.Find(bson.M{
		"running":        true,
		"lockupdatetime": bson.M{"$lt": now.Add(-lockExpireTimeout)},
	}).All(&allData)
	conn.Close()
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error updating expired events")
	}
	for _, evtData := range allData {
		evt := Event{eventData: evtData}
		lastUpdate := evt.LockUpdateTime.UTC()
		err = evt.Done(errors.Errorf("event expired, no update for %v", time.Since(lastUpdate)))
		if err != nil {
			log.Errorf("[events] [event cleaner] error marking evt as done: %v", err)
		} else {
			eventsExpired.WithLabelValues(evt.Kind.Name).Inc()
		}
	}
	return nil
}

func (l *eventCleaner) spin() {
	for {
		err := l.tryCleaning()
		if err != nil {
			log.Errorf("%v", err)
		}
		select {
		case <-l.stopCh:
			return
		case <-time.After(eventCleanerInterval):
		}
	}
}

type lockUpdater struct {
	stopCh chan struct{}
	once   *sync.Once
	setMu  sync.Mutex
	set    map[eventID]struct{}
}

func (l *lockUpdater) start() {
	l.once.Do(func() {
		l.set = make(map[eventID]struct{})
		l.stopCh = make(chan struct{})
		go l.spin()
	})
}

func (l *lockUpdater) stop() {
	if l.stopCh == nil {
		return
	}
	l.stopCh <- struct{}{}
	l.stopCh = nil
	l.once = &sync.Once{}
}

func (l *lockUpdater) add(id eventID) {
	l.setMu.Lock()
	l.set[id] = struct{}{}
	l.setMu.Unlock()
}

func (l *lockUpdater) remove(id eventID) {
	l.setMu.Lock()
	delete(l.set, id)
	l.setMu.Unlock()
}

func (l *lockUpdater) setCopy() map[eventID]struct{} {
	l.setMu.Lock()
	defer l.setMu.Unlock()
	setCopy := make(map[eventID]struct{}, len(l.set))
	for k := range l.set {
		setCopy[k] = struct{}{}
	}
	return setCopy
}

func (l *lockUpdater) spin() {
	for {
		select {
		case <-l.stopCh:
			return
		case <-time.After(lockUpdateInterval):
		}
		set := l.setCopy()
		if len(set) == 0 {
			continue
		}
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("[events] [lock update] error getting db conn: %s", err)
			continue
		}
		coll := conn.Events()
		slice := make([]interface{}, len(set))
		i := 0
		for id := range set {
			slice[i], _ = id.GetBSON()
			i++
		}
		_, err = coll.UpdateAll(bson.M{"_id": bson.M{"$in": slice}}, bson.M{"$set": bson.M{"lockupdatetime": time.Now().UTC()}})
		if err != nil && err != mgo.ErrNotFound {
			log.Errorf("[events] [lock update] error updating: %s", err)
		}
		conn.Close()
	}
}
