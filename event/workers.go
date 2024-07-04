// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/log"
	eventTypes "github.com/tsuru/tsuru/types/event"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	ctx := context.Background()
	now := time.Now().UTC()

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error getting db conn")
	}

	var allData []eventTypes.EventData
	cursor, err := collection.Find(ctx, mongoBSON.M{
		"running":        true,
		"lockupdatetime": mongoBSON.M{"$lt": now.Add(-lockExpireTimeout)},
	})
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error updating expired events")
	}

	err = cursor.All(ctx, &allData)
	if err != nil {
		return errors.Wrap(err, "[events] [event cleaner] error updating expired events")
	}
	for _, evtData := range allData {
		evt := Event{EventData: evtData}
		lastUpdate := evt.LockUpdateTime.UTC()
		err = evt.Done(ctx, errors.Errorf("event expired, no update for %v", time.Since(lastUpdate)))
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
	set    map[primitive.ObjectID]struct{}
}

func (l *lockUpdater) start() {
	l.once.Do(func() {
		l.set = make(map[primitive.ObjectID]struct{})
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

func (l *lockUpdater) add(id primitive.ObjectID) {
	l.setMu.Lock()
	l.set[id] = struct{}{}
	l.setMu.Unlock()
}

func (l *lockUpdater) remove(id primitive.ObjectID) {
	l.setMu.Lock()
	delete(l.set, id)
	l.setMu.Unlock()
}

func (l *lockUpdater) setCopy() map[primitive.ObjectID]struct{} {
	l.setMu.Lock()
	defer l.setMu.Unlock()
	setCopy := make(map[primitive.ObjectID]struct{}, len(l.set))
	for k := range l.set {
		setCopy[k] = struct{}{}
	}
	return setCopy
}

func (l *lockUpdater) spin() {
	ctx := context.Background()
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

		collection, err := storagev2.EventsCollection()

		if err != nil {
			log.Errorf("[events] [lock update] error getting db conn: %s", err)
			continue
		}
		slice := make([]primitive.ObjectID, len(set))
		i := 0
		for id := range set {
			slice[i] = id
			i++
		}
		_, err = collection.UpdateMany(ctx, mongoBSON.M{"_id": mongoBSON.M{"$in": slice}}, mongoBSON.M{"$set": mongoBSON.M{"lockupdatetime": time.Now().UTC()}})
		if err != nil && err != mongo.ErrNoDocuments {
			log.Errorf("[events] [lock update] error updating: %s", err)
		}
	}
}
