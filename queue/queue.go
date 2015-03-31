// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements a Pub/Sub channel in tsuru. It abstracts
// which server is being used and handles connection pooling and
// data transmiting
package queue

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/redisqueue"
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

	// Queue returns a unique redisQueue instance, which will be initialized
	// the first time it's called.
	Queue() (*redisqueue.Queue, error)

	// Resets the queue to a state before initialization
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
