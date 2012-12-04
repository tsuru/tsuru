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
	"io"
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

// ChannelFromWriter returns a channel from a given io.Writer.
//
// Every time a Message is sent to the channel, it gets written to the writer.
// ChannelFromWriter also returns a channel for errors in writtings. You can
// use a select for error checking:
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
