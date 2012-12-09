// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/gob"
	"github.com/globocom/tsuru/queue"
	"net"
	"sync"
)

// FakeQueueServer is a very dumb queue server that does not handle connections
// concurrently.
type FakeQueueServer struct {
	sync.Mutex
	listener net.Listener
	messages []queue.Message
	closed   bool
}

func (s *FakeQueueServer) Start(laddr string) error {
	var err error
	s.listener, err = net.Listen("tcp", laddr)
	if err != nil {
		return err
	}
	go s.loop()
	return nil
}

func (s *FakeQueueServer) loop() {
	for !s.closed {
		conn, err := s.listener.Accept()
		if err != nil {
			if e, ok := err.(*net.OpError); ok && !e.Temporary() {
				return
			}
		}
		decoder := gob.NewDecoder(conn)
		for err == nil {
			var msg queue.Message
			if err := decoder.Decode(&msg); err == nil {
				s.Lock()
				s.messages = append(s.messages, msg)
				s.Unlock()
			}
		}
	}
}

func (s *FakeQueueServer) Stop() error {
	s.closed = true
	return s.listener.Close()
}
