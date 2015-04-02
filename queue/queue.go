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

// QFactory manages queues. It's able to create new queue and handler
// instances.
type QFactory interface {
	// PubSub returns a PubSubQ instance, identified by the given name.
	PubSub(name string) (PubSubQ, error)

	Reset()
}

var factories = map[string]QFactory{
	"redis": &redismqQFactory{},
}

// Register registers a new queue factory. This is how one would add a new
// queue to tsuru.
func Register(name string, factory QFactory) {
	factories[name] = factory
}

// Factory returns an instance of the QFactory used in tsuru. It reads tsuru
// configuration to find the currently used queue system and returns an
// instance of the configured system, if it's registered. Otherwise it
// will return an error.
func Factory() (QFactory, error) {
	name, err := config.GetString("queue")
	if err != nil {
		name = "redis"
	}
	if f, ok := factories[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("Queue %q is not known.", name)
}

var queueInstance monsterqueue.Queue
var queueMutex sync.Mutex

func ResetQueue() {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	if queueInstance != nil {
		queueInstance.Stop()
		queueInstance.ResetStorage()
		queueInstance = nil
	}
}

func Queue() (monsterqueue.Queue, error) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	if queueInstance != nil {
		return queueInstance, nil
	}
	queueStorageUrl, _ := config.GetString("queue:storage")
	if queueStorageUrl == "" {
		queueStorageUrl = "localhost:27017/tsuruqueue"
	}
	conf := mongodb.QueueConfig{
		CollectionPrefix: "tsuru",
		Url:              queueStorageUrl,
	}
	var err error
	queueInstance, err = mongodb.NewQueue(conf)
	if err != nil {
		return nil, err
	}
	go queueInstance.ProcessLoop()
	return queueInstance, nil
}
