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
	content := buf.String()
	return redisQueue.Put(content)
}

func (r *redismqQ) Get(timeout time.Duration) (*Message, error) {
	pack, err := consumer.Get()
	if err != nil {
		return nil, err
	}
	defer pack.Ack()
	reader := strings.NewReader(pack.Payload)
	var msg Message
	if err = json.NewDecoder(reader).Decode(&msg); err != nil && err != io.EOF {
		return nil, fmt.Errorf("Invalid message: %q", pack.Payload)
	}
	return &msg, nil
}

func (r *redismqQ) Delete(m *Message) error {
	return nil
}
