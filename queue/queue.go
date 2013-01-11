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
	"fmt"
	"github.com/globocom/config"
	"github.com/kr/beanstalk"
	"io"
	"regexp"
	"sync"
	"time"
)

// Default TTR for beanstalkd messages.
const ttr = 180e9

var (
	conn           *beanstalk.Conn
	mut            sync.Mutex // for conn access
	timeoutRegexp  = regexp.MustCompile(`TIMED_OUT$`)
	notFoundRegexp = regexp.MustCompile(`not found$`)
)

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

func get(timeout time.Duration, queues ...string) (*Message, error) {
	conn, err := connection()
	if err != nil {
		return nil, err
	}
	ts := beanstalk.NewTubeSet(conn, queues...)
	id, body, err := ts.Reserve(timeout)
	if err != nil {
		if timeoutRegexp.MatchString(err.Error()) {
			return nil, fmt.Errorf("Timed out waiting for message after %s.", timeout)
		}
		return nil, err
	}
	r := bytes.NewReader(body)
	var msg Message
	if err = gob.NewDecoder(r).Decode(&msg); err != nil && err != io.EOF {
		conn.Delete(id)
		return nil, fmt.Errorf("Invalid message: %q", body)
	}
	msg.id = id
	return &msg, nil
}

// Get retrieves a message from the queue.
func Get(queueName string, timeout time.Duration) (*Message, error) {
	return get(timeout, queueName)
}

func connection() (*beanstalk.Conn, error) {
	var (
		addr string
		err  error
	)
	mut.Lock()
	if conn == nil {
		mut.Unlock()
		addr, err = config.GetString("queue-server")
		if err != nil {
			return nil, errors.New(`"queue-server" is not defined in config file.`)
		}
		mut.Lock()
		if conn, err = beanstalk.Dial("tcp", addr); err != nil {
			mut.Unlock()
			return nil, err
		}
	}
	if _, err = conn.ListTubes(); err != nil {
		mut.Unlock()
		conn = nil
		return connection()
	}
	mut.Unlock()
	return conn, err
}
