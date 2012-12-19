// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"encoding/gob"
	"github.com/globocom/tsuru/queue"
	"net"
	"sync"
)

// FakeQueueServer is a very dumb queue server that does not handle connections
// concurrently and stores all messages in an underlying slice.
type FakeQueueServer struct {
	mut      sync.Mutex
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
			if err = decoder.Decode(&msg); err == nil {
				s.mut.Lock()
				s.messages = append(s.messages, msg)
				s.mut.Unlock()
			}
		}
	}
}

func (s *FakeQueueServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *FakeQueueServer) Stop() error {
	s.closed = true
	return s.listener.Close()
}

func (s *FakeQueueServer) Messages() []queue.Message {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.messages
}
