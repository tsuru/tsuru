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
	ping      chan bool
	done      chan bool
	msg       []byte
	lastByte  byte
	running   bool
	writeLock sync.Mutex
}

func NewKeepAliveWriter(w io.Writer, interval time.Duration, msg string) *keepAliveWriter {
	writer := &keepAliveWriter{w: w, interval: interval, msg: append([]byte(msg), '\n')}
	writer.ping = make(chan bool)
	writer.done = make(chan bool)
	writer.running = true
	go writer.keepAlive()
	return writer
}

func (w *keepAliveWriter) writeInterval() {
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
		w.stop()
	} else if numBytes != len(msg) {
		log.Debugf("Short write on keepalive, exiting loop.")
		w.stop()
	}
}

func (w *keepAliveWriter) stop() {
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
			w.writeInterval()
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
		w.ping <- true
	}
	w.lastByte = b[len(b)-1]
	written, err := w.w.Write(b)
	if err != nil {
		w.stop()
	}
	return written, err
}
