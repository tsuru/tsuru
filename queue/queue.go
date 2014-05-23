// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements all the queue handling with tsuru. It abstracts
// which queue server is being used, how the message gets marshaled in to the
// wire and how it's read.
//
// It provides a basic type: Message. You can Put, Get, Delete and Release
// messages, using methods and functions with respective names.
//
// It also provides a generic, thread safe, handler for messages, with start
// and stop capability.
package queue

import (
	"fmt"
	"github.com/tsuru/config"
	"time"
)

// Q represents a queue. A queue is a type that supports the set of
// operations described by this interface.
type Q interface {
	// Get retrieves a message from the queue.
	Get(timeout time.Duration) (*Message, error)

	// Put sends a message to the queue after the given delay. When delay
	// is 0, the message is sent immediately to the queue.
	Put(m *Message, delay time.Duration) error

	Pub(msg []byte) error
	Sub() (chan []byte, error)
	UnSub() error
}

// Handler represents a runnable routine. It can be started and stopped.
type Handler interface {
	// Start starts the handler. It must be safe to call this function
	// multiple times, even if the handler is already running.
	Start()

	// Stop sends a signal to stop the handler, it won't stop the handler
	// immediately. After calling Stop, one should call Wait for blocking
	// until the handler is stopped.
	//
	// This method will return an error if the handler is not running.
	Stop() error

	// Wait blocks until the handler actually stops.
	Wait()
}

// QFactory manages queues. It's able to create new queue and handler
// instances.
type QFactory interface {
	// Get returns a queue instance, identified by the given name.
	Get(name string) (Q, error)

	// Handler returns a handler for the given queue names. Once the
	// handler is started (after calling Start method), it will call f
	// whenever a new message arrives in one of the given queue names.
	Handler(f func(*Message), name ...string) (Handler, error)
}

var factories = map[string]QFactory{
	"beanstalkd": beanstalkdFactory{},
	"redis":      redismqQFactory{},
}

// Register registers a new queue factory. This is how one would add a new
// queue to tsuru.
func Register(name string, factory QFactory) {
	factories[name] = factory
}

// Factory returns an instance of the QFactory used in tsuru. It reads tsuru
// configuration to find the currently used queue system (for example,
// beanstalkd) and returns an instance of the configured system, if it's
// registered. Otherwise it will return an error.
func Factory() (QFactory, error) {
	name, err := config.GetString("queue")
	if err != nil {
		name = "beanstalkd"
	}
	if f, ok := factories[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("Queue %q is not known.", name)
}

// Message represents the message stored in the queue.
//
// A message is specified by an action and a slice of strings, representing
// arguments to the action.
//
// For example, the action "regenerate apprc" could receive one argument: the
// name of the app for which the apprc file will be regenerate.
type Message struct {
	Action string
	Args   []string
	fail   bool
}

// Fail marks the message as failed, telling the handler to requeue it.
func (m *Message) Fail() {
	m.fail = true
}

type timeoutError struct {
	timeout time.Duration
}

func (err *timeoutError) Error() string {
	return fmt.Sprintf("Timed out waiting for message after %s.", err.timeout)
}
