// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"io"
	"sync"
	"time"

	"github.com/tsuru/tsuru/log"
)

type keepAliveWriter struct {
	w         io.Writer
	interval  time.Duration
	ping      chan struct{}
	done      chan struct{}
	msg       []byte
	lastByte  byte
	running   bool
	writeLock sync.Mutex
	limiter   chan struct{}
	// testCh only used for testing
	testCh chan struct{}
}

func NewKeepAliveWriter(w io.Writer, interval time.Duration, msg string) *keepAliveWriter {
	writer := &keepAliveWriter{w: w, interval: interval, msg: append([]byte(msg), '\n')}
	writer.ping = make(chan struct{})
	writer.done = make(chan struct{})
	writer.limiter = make(chan struct{}, 1)
	writer.running = true
	go writer.keepAlive()
	return writer
}

func (w *keepAliveWriter) writeInterval() {
	select {
	case w.limiter <- struct{}{}:
	default:
		return
	}
	defer func() { <-w.limiter }()
	w.writeLock.Lock()
	defer w.writeLock.Unlock()
	msg := []byte{}
	if w.lastByte != '\n' {
		msg = []byte("\n")
	}
	msg = append(msg, w.msg...)
	numBytes, err := w.w.Write(msg)
	if err != nil {
		log.Debugf("Error writing keepalive, exiting loop: %s", err.Error())
		w.Stop()
	} else if numBytes != len(msg) {
		log.Debugf("Short write on keepalive, exiting loop.")
		w.Stop()
	}
	if w.testCh != nil {
		w.testCh <- struct{}{}
	}
}

func (w *keepAliveWriter) Stop() {
	if !w.running {
		return
	}
	w.running = false
	close(w.done)
	close(w.ping)
}

func (w *keepAliveWriter) keepAlive() {
	for {
		select {
		case <-w.ping:
		case <-w.done:
			return
		case <-time.After(w.interval):
			go w.writeInterval()
		}
	}
}

func (w *keepAliveWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	w.writeLock.Lock()
	defer w.writeLock.Unlock()
	if w.running {
		w.ping <- struct{}{}
	}
	w.lastByte = b[len(b)-1]
	written, err := w.w.Write(b)
	if err != nil {
		w.Stop()
	}
	return written, err
}
