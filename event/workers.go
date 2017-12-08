// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	lockUpdateInterval   = 30 * time.Second
	lockExpireTimeout    = 5 * time.Minute
	eventCleanerInterval = 5 * time.Minute
	updater              = lockUpdater{
		addCh:    make(chan eventID, 10),
		removeCh: make(chan eventID, 10),
		once:     &sync.Once{},
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
	defer conn.Close()
	now := time.Now().UTC()
	coll := conn.Events()
	var allData []eventData
	err = coll.Find(bson.M{
		"running":        true,
		"lockupdatetime": bson.M{"$lt": now.Add(-lockExpireTimeout)},
	}).All(&allData)
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error updating expired events")
	}
	for _, evtData := range allData {
		evt := Event{eventData: evtData}
		evt.Init()
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
	addCh    chan eventID
	removeCh chan eventID
	stopCh   chan struct{}
	once     *sync.Once
}

func (l *lockUpdater) start() {
	l.once.Do(func() {
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

func (l *lockUpdater) spin() {
	set := map[eventID]struct{}{}
	for {
		select {
		case added := <-l.addCh:
			set[added] = struct{}{}
		case removed := <-l.removeCh:
			delete(set, removed)
		case <-l.stopCh:
			return
		case <-time.After(lockUpdateInterval):
		}
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
