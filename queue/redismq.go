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
	"github.com/globocom/tsuru/log"
	"io"
	"net"
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

func (factory redismqQFactory) Get(name string) (Q, error) {
	return factory.get(name, "factory")
}

func (redismqQFactory) get(name, consumerName string) (*redismqQ, error) {
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
	consumer, err := queue.AddConsumer(consumerName)
	if err != nil {
		return nil, err
	}
	return &redismqQ{name: name, queue: queue, consumer: consumer}, nil
}

func (factory redismqQFactory) Handler(f func(*Message), names ...string) (Handler, error) {
	name := "default"
	if len(names) > 0 {
		name = names[0]
	}
	consumerName := fmt.Sprintf("handler-%d", time.Now().UnixNano())
	queue, err := factory.get(name, consumerName)
	if err != nil {
		return nil, err
	}
	return &executor{
		inner: func() {
			if message, err := queue.Get(5e9); err == nil {
				log.Debugf("Dispatching %q message to handler function.", message.Action)
				go func(m *Message) {
					f(m)
					if m.fail {
						queue.Put(m, 0)
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
