// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// The size of buffered channels created by ChannelFromWriter.
const ChanSize = 32

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
}

// ChannelFromWriter returns a channel from a given io.WriteCloser.
//
// Every time a Message is sent to the channel, it gets written to the writer
// in gob format.  ChannelFromWriter also returns a channel for errors in
// writtings. You can use a select for error checking:
//
//     ch, errCh := ChannelFromWriter(w)
//     // use ch
//     select {
//     case err := <-errCh:
//         // threat the error
//     case time.After(5e9):
//         // no error after 5 seconds
//     }
//
// Please notice that there is no deadline for the writting. You can obviously
// ignore errors, if they are not significant for you.
//
// Whenever you close the message channel (and you should, to make it clear
// that you will not send any messages to the channel anymore), the error
// channel will get automatically closed, so the WriteCloser.
//
// Both channels are buffered by ChanSize.
func ChannelFromWriter(w io.WriteCloser) (chan<- Message, <-chan error) {
	msgChan := make(chan Message, ChanSize)
	errChan := make(chan error, ChanSize)
	go write(w, msgChan, errChan)
	return msgChan, errChan
}

// write reads messages from ch and write them to w, in gob format.
//
// If clients close ch, write will close errCh.
func write(w io.WriteCloser, ch <-chan Message, errCh chan<- error) {
	defer close(errCh)
	defer w.Close()
	encoder := gob.NewEncoder(w)
	for msg := range ch {
		if err := encoder.Encode(msg); err != nil {
			errCh <- err
		}
	}
}

// Server is the server that hosts the queue. It receives messages and
// process them.
type Server struct {
	listener net.Listener
	messages chan Message
	errors   chan error
	closed   int32
}

// StartServer starts a new queue server from a local address.
//
// The address must be a TCP address, in the format host:port (for example,
// [::1]:8080 or 192.168.254.10:2020).
func StartServer(laddr string) (*Server, error) {
	var (
		server Server
		err    error
	)
	server.listener, err = net.Listen("tcp", laddr)
	if err != nil {
		return nil, errors.New("Could not start server: " + err.Error())
	}
	server.messages = make(chan Message, ChanSize)
	server.errors = make(chan error, ChanSize)
	go server.loop()
	return &server, nil
}

// handle handles a new client, sending errors to the qs.errors channel.
func (qs *Server) handle(conn net.Conn) {
	var err error
	decoder := gob.NewDecoder(conn)
	for err == nil {
		var msg Message
		if err = decoder.Decode(&msg); err == nil {
			qs.messages <- msg
		} else if atomic.LoadInt32(&qs.closed) == 0 {
			qs.errors <- err
		}
	}
}

// loop accepts connection forever, and uses read to read messages from it,
// decoding them to a channel of messages.
func (qs *Server) loop() {
	for atomic.LoadInt32(&qs.closed) == 0 {
		conn, err := qs.listener.Accept()
		if err != nil {
			if e, ok := err.(*net.OpError); ok && !e.Temporary() {
				return
			}
			continue
		}
		go qs.handle(conn)
	}
}

// Message returns the first available message in the queue, or an error if it
// fails to read the message, or times out while waiting for the message.
//
// If timeout is negative, this method will wait nearly forever for the
// arriving of a message or an error.
func (qs *Server) Message(timeout time.Duration) (Message, error) {
	var (
		msg Message
		err error
	)
	if timeout < 0 {
		timeout = 1 << 62
	}
	select {
	case msg = <-qs.messages:
	case err = <-qs.errors:
		if err == io.EOF {
			err = errors.New("EOF: client disconnected.")
		}
	case <-time.After(timeout):
		err = errors.New("Timed out waiting for the message.")
	}
	return msg, err
}

// PutBack puts a message back in the queue. It should be used when a message
// got using Message method cannot be processed yet. You put it back in the
// queue for processing later.
func (qs *Server) PutBack(message Message) {
	qs.messages <- message
}

// Addr returns the address of the server.
func (qs *Server) Addr() string {
	return qs.listener.Addr().String()
}

// Close closes the server, closing the underlying listener.
func (qs *Server) Close() error {
	if !atomic.CompareAndSwapInt32(&qs.closed, 0, 1) {
		return errors.New("Server already closed.")
	}
	err := qs.listener.Close()
	close(qs.messages)
	close(qs.errors)
	return err
}

// Dial is used to connect to a queue server.
//
// It returns three values: the channel to which messages should be sent, the
// channel where the client will get errors from the server during writing of
// messages and an error, that will be non-nil in case of failure to connect to
// the queue server.
//
// Whenever the message channel gets closed, the connection with the remote
// server will be closed.
func Dial(addr string) (chan<- Message, <-chan error, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, errors.New("Could not dial to " + addr + ": " + err.Error())
	}
	messages, errors := ChannelFromWriter(conn)
	return messages, errors, nil
}
