// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/gob"
	. "launchpad.net/gocheck"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

// SafeBuffer is a thread safe buffer.
type SafeBuffer struct {
	closed int32
	buf    bytes.Buffer
	sync.Mutex
}

func (sb *SafeBuffer) Read(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) Write(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Write(p)
}

func (sb *SafeBuffer) Close() error {
	atomic.StoreInt32(&sb.closed, 1)
	return nil
}

func (s *S) TestChannelFromWriter(c *C) {
	var buf SafeBuffer
	message := Message{
		Action: "delete",
		Args:   []string{"everything"},
	}
	ch, _ := ChannelFromWriter(&buf)
	ch <- message
	time.Sleep(1e6)
	var decodedMessage Message
	decoder := gob.NewDecoder(&buf)
	err := decoder.Decode(&decodedMessage)
	c.Assert(err, IsNil)
	c.Assert(decodedMessage, DeepEquals, message)
}

func (s *S) TestClosesErrChanWhenClientCloseMessageChannel(c *C) {
	var buf SafeBuffer
	ch, errCh := ChannelFromWriter(&buf)
	close(ch)
	_, ok := <-errCh
	c.Assert(ok, Equals, false)
}

func (s *S) TestClosesWriteCloserWhenClientClosesMessageChannel(c *C) {
	var buf SafeBuffer
	ch, _ := ChannelFromWriter(&buf)
	close(ch)
	time.Sleep(1e6)
	c.Assert(atomic.LoadInt32(&buf.closed), Equals, int32(1))
}

func (s *S) TestWriteSendErrorsInTheErrorChannel(c *C) {
	messages := make(chan Message, 1)
	errCh := make(chan error, 1)
	conn := NewFakeConn("127.0.0.1:2345", "127.0.0.1:12345")
	conn.Close()
	go write(conn, messages, errCh)
	messages <- Message{}
	close(messages)
	err, ok := <-errCh
	c.Assert(ok, Equals, true)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Closed connection.")
}

func (s *S) TestReadSendErrorsInTheErrorChannel(c *C) {
	messages := make(chan Message, 1)
	errChan := make(chan error, 1)
	conn := NewFakeConn("127.0.0.1:5055", "127.0.0.1:8080")
	conn.Close()
	go read(conn, messages, errChan)
	err := <-errChan
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Closed connection.")
}

func (s *S) TestServerAddr(c *C) {
	listener := NewFakeListener("0.0.0.0:8000")
	server := Server{listener: listener}
	c.Assert(server.Addr(), Equals, listener.Addr().String())
}

func (s *S) TestStartServerAndReadMessage(c *C) {
	message := Message{
		Action: "delete",
		Args:   []string{"something"},
	}
	server, err := StartServer("127.0.0.1:0")
	c.Assert(err, IsNil)
	defer server.Close()
	conn, err := net.Dial("tcp", server.Addr())
	c.Assert(err, IsNil)
	defer conn.Close()
	encoder := gob.NewEncoder(conn)
	err = encoder.Encode(message)
	c.Assert(err, IsNil)
	gotMessage, err := server.Message(2e9)
	c.Assert(err, IsNil)
	c.Assert(gotMessage, DeepEquals, message)
}

func (s *S) TestMessageNegativeTimeout(c *C) {
	server := Server{
		messages: make(chan Message, 1),
		errors:   make(chan error, 1),
	}
	var (
		got, want Message
		err       error
		wg        sync.WaitGroup
	)
	want = Message{Action: "create"}
	wg.Add(1)
	go func() {
		got, err = server.Message(-1)
		wg.Done()
	}()
	time.Sleep(1e6)
	server.messages <- want
	wg.Wait()
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}

func (s *S) TestPutBack(c *C) {
	server := Server{
		messages: make(chan Message, 1),
		errors:   make(chan error, 1),
	}
	want := Message{Action: "delete"}
	server.PutBack(want)
	got, err := server.Message(1e6)
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}

func (s *S) TestDontHangWhenClientClosesTheConnection(c *C) {
	server, err := StartServer("127.0.0.1:0")
	c.Assert(err, IsNil)
	defer server.Close()
	messages, _, err := Dial(server.Addr())
	c.Assert(err, IsNil)
	close(messages)
	msg, err := server.Message(1e6)
	c.Assert(msg, DeepEquals, Message{})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "EOF: client disconnected.")
}

func (s *S) TestDial(c *C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, IsNil)
	defer listener.Close()
	received := make(chan Message, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			decoder := gob.NewDecoder(conn)
			var message Message
			if err = decoder.Decode(&message); err != nil {
				panic(err)
			}
			received <- message
		}
	}()
	sent := Message{
		Action: "delete",
		Args:   []string{"everything"},
	}
	messages, _, err := Dial(listener.Addr().String())
	c.Assert(err, IsNil)
	messages <- sent
	got := <-received
	c.Assert(got, DeepEquals, sent)
}
