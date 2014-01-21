// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/adeven/redismq"
	"github.com/globocom/config"
	"io"
	"strings"
	"time"
)

type redismqQ struct {
	name     string
	queue    *redismq.Queue
	consumer *redismq.Consumer
}

func (r *redismqQ) Put(m *Message, delay time.Duration) error {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(m)
	if err != nil {
		return err
	}
	if delay > 0 {
		go func() {
			time.Sleep(delay)
			r.queue.Put(buf.String())
		}()
		return nil
	} else {
		return r.queue.Put(buf.String())
	}
}

func (r *redismqQ) Get(timeout time.Duration) (*Message, error) {
	packChan := make(chan *redismq.Package)
	errChan := make(chan error)
	quit := make(chan int)
	go func() {
		var pack *redismq.Package
		var err error
		for pack == nil {
			select {
			case <-quit:
				return
			default:
				pack, err = r.consumer.NoWaitGet()
				if err != nil {
					errChan <- err
					return
				}
			}
		}
		packChan <- pack
	}()
	var pack *redismq.Package
	select {
	case pack = <-packChan:
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		close(quit)
		return nil, &timeoutError{timeout: timeout}
	}
	defer pack.Ack()
	reader := strings.NewReader(pack.Payload)
	var msg Message
	if err := json.NewDecoder(reader).Decode(&msg); err != nil && err != io.EOF {
		return nil, fmt.Errorf("Invalid message: %q", pack.Payload)
	}
	return &msg, nil
}

type redismqQFactory struct{}

func (redismqQFactory) Get(name string) (Q, error) {
	host, err := config.GetString("queue:redis-host")
	if err != nil {
		host = "localhost"
	}
	port, err := config.GetString("queue:redis-port")
	if err != nil {
		if nport, err := config.GetInt("queue:redis-port"); err != nil {
			port = "6379"
		} else {
			port = fmt.Sprintf("%d", nport)
		}
	}
	password, _ := config.GetString("queue:redis-password")
	db, err := config.GetInt("queue:redis-db")
	if err != nil {
		db = 3
	}
	queue := redismq.CreateQueue(host, port, password, int64(db), name)
	consumer, err := queue.AddConsumer("factory")
	if err != nil {
		return nil, err
	}
	return &redismqQ{name: name, queue: queue, consumer: consumer}, nil
}

func (redismqQFactory) Handler(f func(*Message), names ...string) (Handler, error) {
	return nil, nil
}
