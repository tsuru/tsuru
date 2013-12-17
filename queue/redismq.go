// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/json"
	// "errors"
	"fmt"
	// "github.com/globocom/config"
	"github.com/adeven/redismq"
	"github.com/globocom/tsuru/log"
	"io"
	// "net"
	// "regexp"
	// "sync"
	"strings"
	"time"
	// "fmt"
)

type redismqQ struct {
	name string
}

var redisQueue = redismq.CreateQueue("localhost", "6379", "", 9, "clicks")
var consumer *redismq.Consumer

func init() {
	var err error
	consumer, err = redisQueue.AddConsumer("testconsumer")
	if err != nil {
		log.Errorf("Failed to create the consumer: %s", err)
	}
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
			redisQueue.Put(buf.String())
		}()
		return nil
	} else {
		return redisQueue.Put(buf.String())
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
				pack, err = consumer.NoWaitGet()
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

func (r *redismqQ) Delete(m *Message) error {
	return nil
}

func (r *redismqQ) Release(m *Message, delay time.Duration) error {
	return r.Put(m, delay)
}
