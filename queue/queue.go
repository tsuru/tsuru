// Copyright 2013 tsuru authors. All rights reserved.
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

import "time"

// Queue represents a queue. A queue is a type that supports the set of
// operations described by this interface.
type Queue interface {
	// Get retrieves a message from the queue.
	Get(timeout time.Duration) (*Message, error)

	// Put sends a message to the queue after the given delay. When delay
	// is 0, the message is sent immediately to the queue.
	Put(m *Message, delay time.Duration) error

	// Delete deletes a message from the queue.
	Delete(m *Message) error

	// Release puts a message back in the queue the given delay. When delay
	// is 0, the message is released immediately.
	//
	// This method should be used when handling a message that you cannot
	// handle, maximizing throughput.
	Release(m *Message, delay time.Duration) error
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
	Get(name string) (Queue, error)

	// Handler returns a handler for the given queue names. Once the
	// handler is started (after calling Start method), it will call f
	// whenever a new message arrives in one of the given queue names.
	Handler(f func(*Message), name ...string) (Handler, error)
}

// Factory returns an instance of the QFactory used in tsuru. It reads tsuru
// configuration to find the currently used queue system (for example,
// beanstalk) and returns an instance of the configured system, if it's
// registered. Otherwise it will return an error.
func Factory() (QFactory, error) {
	return nil, nil
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
	id     uint64
	delete bool
}

// Delete deletes the message from the queue.
func (m *Message) Delete() {
	m.delete = true
}
