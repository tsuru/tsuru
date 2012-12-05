// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package netqueue implements a queue based on channels and networking.
//
// It is based on concepts from old/netchan and a lot of discussion about this
// theme on the internet. The implementation present here is specific to tsuru,
// but could be more generic.
package netqueue

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
	"time"
)

// The size of buffered channels created by ChannelFromReader and
// ChannelFromWriter.
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

// ChannelFromWriter returns a channel from a given io.Writer.
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
// that you will not send any messages to the channel anymore), error channel
// will get automatically closed.
//
// Both channels are buffered by ChanSize.
func ChannelFromWriter(w io.Writer) (chan<- Message, <-chan error) {
	msgChan := make(chan Message, ChanSize)
	errChan := make(chan error, ChanSize)
	go write(w, msgChan, errChan)
	return msgChan, errChan
}

// write reads messages from ch and write them to w, in gob format.
//
// If clients close ch, write will close errCh.
func write(w io.Writer, ch <-chan Message, errCh chan<- error) {
	defer close(errCh)
	for msg := range ch {
		encoder := gob.NewEncoder(w)
		if err := encoder.Encode(msg); err != nil {
			errCh <- err
		}
	}
}

// ChannelFromReader returns a channel from a given io.Reader.
//
// Every time a chunk of gobs is read from r, it will be decoded to a Message
// and sent to the message channel. ChannelFromReader also returns another
// channel for errors in reading. You can use a select for reading messages or
// errors:
//
//     ch, errCh := ChannelFromReader(r)
//     select {
//     case msg := <-ch:
//         // Do something with msg
//     case err := <-errCh:
//         // Threat the error
//     }
//
// If the reading or decoding fail for any reason, the error will be sent to
// the error channels and both channels will be closed.
func ChannelFromReader(r io.Reader) (<-chan Message, <-chan error) {
	msgCh := make(chan Message, ChanSize)
	errCh := make(chan error, ChanSize)
	go read(r, msgCh, errCh)
	return msgCh, errCh
}

// read reads bytes from r, decode these bytes as Message's and send each
// message to ch.
//
// Any error on reading will be sen to errCh (except io.EOF).
func read(r io.Reader, ch chan<- Message, errCh chan<- error) {
	var err error
	decoder := gob.NewDecoder(r)
	for err == nil {
		var msg Message
		if err = decoder.Decode(&msg); err == nil {
			ch <- msg
		} else if err != io.EOF {
			errCh <- err
		}
	}
	close(ch)
	close(errCh)
}

// Server is the server that hosts the queue. It receives messages and
// process them.
type Server struct {
	listener net.Listener
	messages chan Message
	errors   chan error
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
	server.messages = make(chan Message, ChanSize)
	server.errors = make(chan error, ChanSize)
	if err != nil {
		return nil, errors.New("Could not start server: " + err.Error())
	}
	go server.loop()
	return &server, nil
}

// loop accepts connection forever, and uses read to read messages from it,
// decoding them to a channel of messages.
func (qs *Server) loop() {
	for {
		conn, err := qs.listener.Accept()
		if err != nil {
			if e, ok := err.(*net.OpError); ok && !e.Temporary() {
				return
			}
			continue
		}
		go read(conn, qs.messages, qs.errors)
	}
}

// Message returns the first available message in the queue, or an error if it
// fails to read the message, or times out while waiting for the message.
//
// If timeout is negative, this method will wait forever for the message.
func (qs *Server) Message(timeout time.Duration) (Message, error) {
	var (
		msg Message
		err error
	)
	if timeout < 0 {
		timeout = 365 * 30 * 24 * time.Hour
	}
	select {
	case msg = <-qs.messages:
	case err = <-qs.errors:
	case <-time.After(timeout):
		err = errors.New("Timed out waiting for the message.")
	}
	return msg, err
}

// Addr returns the address of the server.
func (qs *Server) Addr() string {
	return qs.listener.Addr().String()
}

// Close closes the server listener.
func (qs *Server) Close() error {
	return qs.listener.Close()
}
