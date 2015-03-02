// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuetest

import (
	"sync"
	"time"

	"github.com/tsuru/tsuru/queue"
)

var factory = NewFakePubSubQFactory()

func init() {
	queue.Register("fake", factory)
}

type FakePubSubQ struct {
	messages       messageQueue
	name           string
	pubSubStop     chan bool
	pubSubStopLock sync.Mutex
}

type Message struct {
	Action string
}

type SyncSet struct {
	set map[string]bool
	sync.Mutex
}

var subscribersSet = SyncSet{set: make(map[string]bool)}

func (s *SyncSet) put(val string) {
	s.Lock()
	defer s.Unlock()
	s.set[val] = true
}

func (s *SyncSet) get(val string) bool {
	s.Lock()
	defer s.Unlock()
	return s.set[val]
}

func (s *SyncSet) delete(val string) {
	s.Lock()
	defer s.Unlock()
	delete(s.set, val)
}

func (q *FakePubSubQ) Pub(msg []byte) error {
	q.pubSubStopLock.Lock()
	if q.pubSubStop == nil {
		q.pubSubStop = make(chan bool)
	}
	q.pubSubStopLock.Unlock()
	m := Message{Action: string(msg)}
	q.messages.enqueue(&m)
	return nil
}

func (q *FakePubSubQ) Sub() (chan []byte, error) {
	q.pubSubStopLock.Lock()
	if q.pubSubStop == nil {
		q.pubSubStop = make(chan bool)
	}
	q.pubSubStopLock.Unlock()
	subChan := make(chan []byte)
	go func() {
		defer close(subChan)
		for {
			q.pubSubStopLock.Lock()
			select {
			case <-q.pubSubStop:
				q.pubSubStopLock.Unlock()
				return
			default:
			}
			q.pubSubStopLock.Unlock()
			if msg := q.messages.dequeue(); msg != nil {
				subChan <- []byte(msg.Action)
			}
			time.Sleep(1e3)
		}
	}()
	subscribersSet.put(q.name)
	return subChan, nil
}

func (q *FakePubSubQ) UnSub() error {
	subscribersSet.delete(q.name)
	q.pubSubStopLock.Lock()
	close(q.pubSubStop)
	q.pubSubStopLock.Unlock()
	return nil
}

type FakePubSubQFactory struct {
	queues map[string]*FakePubSubQ
	sync.Mutex
}

func NewFakePubSubQFactory() *FakePubSubQFactory {
	return &FakePubSubQFactory{
		queues: make(map[string]*FakePubSubQ),
	}
}

func (f *FakePubSubQFactory) Get(name string) (queue.PubSubQ, error) {
	f.Lock()
	defer f.Unlock()
	if q, ok := f.queues[name]; ok {
		return q, nil
	}
	q := FakePubSubQ{name: name}
	f.queues[name] = &q
	return &q, nil
}

func (f *FakePubSubQFactory) Reset() {
	f.Lock()
	defer f.Unlock()
	f.queues = make(map[string]*FakePubSubQ)
}

type messageNode struct {
	m    *Message
	next *messageNode
	prev *messageNode
}

type messageQueue struct {
	first *messageNode
	last  *messageNode
	n     int
	sync.Mutex
}

func (q *messageQueue) enqueue(msg *Message) {
	q.Lock()
	defer q.Unlock()
	if q.last == nil {
		q.last = &messageNode{m: msg}
		q.first = q.last
	} else {
		olast := q.last
		q.last = &messageNode{m: msg, prev: olast}
		olast.next = q.last
	}
	q.n++
}

func (q *messageQueue) dequeue() *Message {
	q.Lock()
	defer q.Unlock()
	if q.n == 0 {
		return nil
	}
	msg := q.first.m
	q.n--
	q.first = q.first.next
	if q.n == 0 {
		q.last = q.first
	}
	return msg
}
