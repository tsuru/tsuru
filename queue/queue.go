// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements all the queue handling with tsuru. It abstract
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
	"bytes"
	"encoding/gob"
	"errors"
	"github.com/kr/beanstalk"
	"time"
)

// Queue represents a queue. A queue is a type that supports the set of
// operations described by this interface.
type Queue interface {
	// Get retrieves a message from the queue.
	Get(timeout time.Duration) (*Message, error)

	// Put sends a message to the queue after delay time. To send the
	// message immediately, just set delay to 0.
	Put(m *Message, delay time.Duration) error

	// Delete deletes a message from the queue.
	Delete(m *Message) error

	// Release puts a message back in the queue after a delay. To release
	// the message immediately, just set delay to 0.
	//
	// This method should be used when handling a message that you cannot
	// handle, maximizing throughput.
	Release(m *Message, delay time.Duration) error
}

// QFactory manages queues. It's able to create new queue and handler
// instances.
type QFactory interface {
	// Get returns a queue instance, identified by the given name.
	Get(name string) (Queue, error)

	// Handler returns a handler for the given queue names.
	Handler(f func(Queue, *Message), name ...string) (*Handler, error)
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
}

// Release puts a message back in the queue after a delay. To release the
// message immediately, just set delay to 0.
//
// This method should be used when handling a message that you cannot handle,
// maximizing throughput.
func (msg *Message) Release(delay time.Duration) error {
	if msg.id == 0 {
		return errors.New("Unknown message.")
	}
	conn, err := connection()
	if err != nil {
		return err
	}
	if err = conn.Release(msg.id, 1, delay); err != nil && notFoundRegexp.MatchString(err.Error()) {
		return errors.New("Message not found.")
	}
	return err
}

// Delete deletes the message from the queue. For deletion, the message must be
// one returned by Get, or added by Put. This function uses internal state of
// the message to delete it.
func (msg *Message) Delete() error {
	conn, err := connection()
	if err != nil {
		return err
	}
	if msg.id == 0 {
		return errors.New("Unknown message.")
	}
	if err = conn.Delete(msg.id); err != nil && notFoundRegexp.MatchString(err.Error()) {
		return errors.New("Message not found.")
	}
	return err
}

// Put sends the message to the queue after delay time. To send the message
// immediately, just set delay to 0.
func (msg *Message) Put(queueName string, delay time.Duration) error {
	conn, err := connection()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(msg)
	if err != nil {
		return err
	}
	tube := beanstalk.Tube{Conn: conn, Name: queueName}
	id, err := tube.Put(buf.Bytes(), 1, delay, ttr)
	msg.id = id
	return err
}

// Get retrieves a message from the queue.
func Get(queueName string, timeout time.Duration) (*Message, error) {
	return get(timeout, queueName)
}
