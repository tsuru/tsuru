// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/log"
	"github.com/kr/beanstalk"
	"io"
	"net"
	"regexp"
	"sync"
	"time"
)

// Default TTR for beanstalkd messages.
const ttr = 180e9

var (
	conn           *beanstalk.Conn
	mut            sync.Mutex // for conn access
	timeoutRegexp  = regexp.MustCompile(`(TIMED_OUT|timeout)$`)
	notFoundRegexp = regexp.MustCompile(`not found$`)
)

type beanstalkdQ struct {
	name string
}

func (b *beanstalkdQ) Get(timeout time.Duration) (*Message, error) {
	return get(timeout, b.name)
}

func (b *beanstalkdQ) Put(m *Message, delay time.Duration) error {
	conn, err := connection()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(m)
	if err != nil {
		return err
	}
	tube := beanstalk.Tube{Conn: conn, Name: b.name}
	_, err = tube.Put(buf.Bytes(), 1, delay, ttr)
	return err
}

type beanstalkdFactory struct{}

func (b beanstalkdFactory) Get(name string) (Q, error) {
	return &beanstalkdQ{name: name}, nil
}

func (b beanstalkdFactory) Handler(f func(*Message), names ...string) (Handler, error) {
	name := "default"
	if len(names) > 0 {
		name = names[0]
	}
	return &executor{
		inner: func() {
			if message, err := get(5e9, names...); err == nil {
				log.Debugf("Dispatching %q message to handler function.", message.Action)
				go func(m *Message) {
					f(m)
					if m.fail {
						q := beanstalkdQ{name: name}
						q.Put(m, 0)
					}
				}(message)
			} else {
				log.Debugf("Failed to get message from the queue: %s. Trying again...", err)
				if e, ok := err.(*net.OpError); ok && e.Op == "dial" {
					time.Sleep(5e9)
				}
			}
		},
	}, nil
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
			addr = "localhost:11300"
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

func get(timeout time.Duration, queues ...string) (*Message, error) {
	conn, err := connection()
	if err != nil {
		return nil, err
	}
	ts := beanstalk.NewTubeSet(conn, queues...)
	id, body, err := ts.Reserve(timeout)
	if err != nil {
		if timeoutRegexp.MatchString(err.Error()) {
			return nil, &timeoutError{timeout: timeout}
		}
		return nil, err
	}
	defer conn.Delete(id)
	r := bytes.NewReader(body)
	var msg Message
	if err = gob.NewDecoder(r).Decode(&msg); err != nil && err != io.EOF {
		return nil, fmt.Errorf("Invalid message: %q", body)
	}
	return &msg, nil
}
