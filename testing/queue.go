// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"github.com/tsuru/tsuru/queue"
	"sync"
	"sync/atomic"
	"time"
)

var factory = NewFakeQFactory()

func init() {
	queue.Register("fake", factory)
}

type fakeHandler struct {
	running int32
}

func (h *fakeHandler) Start() {
	atomic.StoreInt32(&h.running, 1)
}

func (h *fakeHandler) Stop() error {
	if !atomic.CompareAndSwapInt32(&h.running, 1, 0) {
		return errors.New("Not running.")
	}
	return nil
}

func (h *fakeHandler) Wait() {}

type FakeQ struct {
	messages   messageQueue
	pubSubStop chan int
	name       string
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

func (q *FakeQ) get(ch chan *queue.Message, stop chan int) {
	defer close(ch)
	for {
		select {
		case <-stop:
			return
		default:
		}
		if msg := q.messages.dequeue(); msg != nil {
			ch <- msg
			return
		}
		time.Sleep(1e3)
	}
}

func (q *FakeQ) Pub(msg []byte) error {
	if !subscribersSet.get(q.name) {
		return nil
	}
	m := queue.Message{Action: string(msg)}
	q.messages.enqueue(&m)
	return nil
}

func (q *FakeQ) Sub() (chan []byte, error) {
	subChan := make(chan []byte)
	q.pubSubStop = make(chan int, 1)
	go func() {
		defer close(subChan)
		for {
			select {
			case <-q.pubSubStop:
				return
			default:
			}
			if msg := q.messages.dequeue(); msg != nil {
				subChan <- []byte(msg.Action)
			}
			time.Sleep(1e3)
		}
	}()
	subscribersSet.put(q.name)
	return subChan, nil
}

func (q *FakeQ) UnSub() error {
	subscribersSet.delete(q.name)
	close(q.pubSubStop)
	return nil
}

func (q *FakeQ) Get(timeout time.Duration) (*queue.Message, error) {
	ch := make(chan *queue.Message, 1)
	stop := make(chan int, 1)
	defer close(stop)
	go q.get(ch, stop)
	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
	}
	return nil, errors.New("Timed out.")
}

func (q *FakeQ) Put(m *queue.Message, delay time.Duration) error {
	if delay > 0 {
		go func() {
			time.Sleep(delay)
			q.messages.enqueue(m)
		}()
	} else {
		q.messages.enqueue(m)
	}
	return nil
}

type FakeQFactory struct {
	queues map[string]*FakeQ
	sync.Mutex
}

func NewFakeQFactory() *FakeQFactory {
	return &FakeQFactory{
		queues: make(map[string]*FakeQ),
	}
}

func (f *FakeQFactory) Get(name string) (queue.Q, error) {
	f.Lock()
	defer f.Unlock()
	if q, ok := f.queues[name]; ok {
		return q, nil
	}
	q := FakeQ{name: name}
	f.queues[name] = &q
	return &q, nil
}

func (f *FakeQFactory) Handler(fn func(*queue.Message), names ...string) (queue.Handler, error) {
	return &fakeHandler{}, nil
}

type messageNode struct {
	m    *queue.Message
	next *messageNode
	prev *messageNode
}

type messageQueue struct {
	first *messageNode
	last  *messageNode
	n     int
	sync.Mutex
}

func (q *messageQueue) enqueue(msg *queue.Message) {
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

func (q *messageQueue) dequeue() *queue.Message {
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

// CleanQ deletes all messages from queues identified by the given names.
func CleanQ(names ...string) {
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			q, _ := factory.Get(name)
			for {
				_, err := q.Get(1e6)
				if err != nil {
					break
				}
			}
		}(name)
	}
	wg.Wait()
}
