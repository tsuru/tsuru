// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"errors"
	"github.com/tsuru/tsuru/log"
	"io"
	"sync"
	"time"
)

type keepAliveWriter struct {
	w         io.Writer
	interval  time.Duration
	ping      chan bool
	done      chan bool
	msg       []byte
	lastByte  byte
	withError bool
	writeLock sync.Mutex
}

func NewKeepAliveWriter(w io.Writer, interval time.Duration, msg string) *keepAliveWriter {
	writer := &keepAliveWriter{w: w, interval: interval, msg: append([]byte(msg), '\n')}
	writer.ping = make(chan bool)
	writer.done = make(chan bool)
	go writer.keepAlive()
	return writer
}

func (w *keepAliveWriter) writeInterval() {
	w.writeLock.Lock()
	defer func() {
		w.writeLock.Unlock()
	}()
	msg := []byte{}
	if w.lastByte != '\n' {
		msg = []byte("\n")
	}
	msg = append(msg, w.msg...)
	numBytes, err := w.w.Write(msg)
	if err != nil {
		log.Debugf("Error writing keepalive, exiting loop: %s", err.Error())
		w.withError = true
		return
	}
	if numBytes != len(msg) {
		log.Debugf("Short write on keepalive, exiting loop.")
		w.withError = true
		return
	}
}

func (w *keepAliveWriter) keepAlive() {
	for {
		select {
		case <-w.ping:
		case <-w.done:
			return
		case <-time.After(w.interval):
			if w.writeInterval(); w.withError {
				return
			}
		}
	}
}

func (w *keepAliveWriter) Write(b []byte) (int, error) {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()
	if w.withError {
		return 0, errors.New("Error in previous write.")
	}
	w.ping <- true
	w.lastByte = b[len(b)-1]
	written, err := w.w.Write(b)
	if err != nil {
		close(w.done)
	}
	return written, err
}
