// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements a Pub/Sub channel in tsuru. It abstracts
// which server is being used and handles connection pooling and
// data transmiting
package queue

import (
	"fmt"
	"sync"

	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/monsterqueue/mongodb"
	"github.com/tsuru/tsuru/api/shutdown"
)

// PubSubQ represents an implementation that allows Publishing and
// Subscribing messages.
type PubSubQ interface {
	// Publishes a message using the underlaying queue server.
	Pub(msg []byte) error

	// Returns a channel that will yield every message published to this
	// queue.
	Sub() (chan []byte, error)

	// Unsubscribe the queue, this should make sure the channel returned
	// by Sub() is closed.
	UnSub() error
}

// PubSubFactory manages queues. It's able to create new queue and handler
// instances.
type PubSubFactory interface {
	// PubSub returns a PubSubQ instance, identified by the given name.
	PubSub(name string) (PubSubQ, error)

	Reset()
}

var factories = map[string]PubSubFactory{
	"redis": &redisPubSubFactory{},
}

// Register registers a new queue factory. This is how one would add a new
// queue to tsuru.
func Register(name string, factory PubSubFactory) {
	factories[name] = factory
}

// Factory returns an instance of the PubSubFactory used in tsuru. It reads tsuru
// configuration to find the currently used queue system and returns an
// instance of the configured system, if it's registered. Otherwise it
// will return an error.
func Factory() (PubSubFactory, error) {
	name, err := config.GetString("queue")
	if err != nil {
		name = "redis"
	}
	if f, ok := factories[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("Queue %q is not known.", name)
}

type queueInstanceData struct {
	sync.Mutex
	instance monsterqueue.Queue
}

func (q *queueInstanceData) Shutdown() {
	q.Lock()
	defer q.Unlock()
	if q.instance != nil {
		q.instance.Stop()
		q.instance = nil
	}
}

func (q *queueInstanceData) String() string {
	return "queued tasks"
}

var queueData queueInstanceData

func ResetQueue() {
	queueData.Lock()
	defer queueData.Unlock()
	if queueData.instance != nil {
		queueData.instance.Stop()
		queueData.instance.ResetStorage()
		queueData.instance = nil
	}
}

func Queue() (monsterqueue.Queue, error) {
	queueData.Lock()
	defer queueData.Unlock()
	if queueData.instance != nil {
		return queueData.instance, nil
	}
	queueMongoUrl, _ := config.GetString("queue:mongo-url")
	if queueMongoUrl == "" {
		queueMongoUrl = "localhost:27017"
	}
	queueMongoDB, _ := config.GetString("queue:mongo-database")
	conf := mongodb.QueueConfig{
		CollectionPrefix: "tsuru",
		Url:              queueMongoUrl,
		Database:         queueMongoDB,
	}
	var err error
	queueData.instance, err = mongodb.NewQueue(conf)
	if err != nil {
		return nil, fmt.Errorf("could not create queue instance, please check queue:mongo-url and queue:mongo-database config entries. error: %s", err)
	}
	shutdown.Register(&queueData)
	go queueData.instance.ProcessLoop()
	return queueData.instance, nil
}
