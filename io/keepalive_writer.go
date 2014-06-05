// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"github.com/tsuru/tsuru/log"
	"io"
	"time"
)

type keepAliveWriter struct {
	w        io.Writer
	interval time.Duration
	ping     chan bool
	done     chan bool
	msg      []byte
	lastByte byte
}

func NewKeepAliveWriter(w io.Writer, interval time.Duration, msg string) *keepAliveWriter {
	writer := &keepAliveWriter{w: w, interval: interval, msg: append([]byte(msg), '\n')}
	writer.ping = make(chan bool)
	go writer.keepAlive()
	return writer
}

func (w *keepAliveWriter) keepAlive() {
	for {
		select {
		case <-w.ping:
		case <-w.done:
			return
		case <-time.After(w.interval):
			msg := []byte{}
			if w.lastByte != '\n' {
				msg = []byte("\n")
			}
			msg = append(msg, w.msg...)
			numBytes, err := w.w.Write(msg)
			if err != nil {
				log.Debugf("Error writing keepalive, exiting loop: %s", err.Error())
				return
			}
			if numBytes != len(msg) {
				log.Debugf("Short write on keepalive, exiting loop.")
				return
			}
		}
	}
}

func (w *keepAliveWriter) Write(b []byte) (int, error) {
	w.ping <- true
	w.lastByte = b[len(b)-1]
	written, err := w.w.Write(b)
	if err != nil {
		w.done <- true
	}
	return written, err
}
