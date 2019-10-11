// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	_ WriterFlusher = &FlushingWriter{}
	_ http.Hijacker = &FlushingWriter{}
)

type WriterFlusher interface {
	http.ResponseWriter
	http.Flusher
}

// FlushingWriter is a custom writer that flushes after writing, if the
// underlying ResponseWriter is also an http.Flusher.
type FlushingWriter struct {
	WriterFlusher
	MaxLatency   time.Duration
	writeMutex   sync.Mutex
	timer        *time.Timer
	wrote        bool
	flushPending bool
	hijacked     bool
	closed       bool
}

func (w *FlushingWriter) WriteHeader(code int) {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	w.wrote = true
	w.WriterFlusher.WriteHeader(code)
}

// Write writes and flushes the data.
func (w *FlushingWriter) Write(data []byte) (written int, err error) {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	if w.closed {
		return 0, io.EOF
	}
	w.wrote = true
	written, err = w.WriterFlusher.Write(data)
	if err != nil {
		return
	}
	if w.MaxLatency == 0 {
		w.WriterFlusher.Flush()
		return
	}
	if w.flushPending {
		return
	}
	w.flushPending = true
	if w.timer == nil {
		w.timer = time.AfterFunc(w.MaxLatency, w.delayedFlush)
	} else {
		w.timer.Reset(w.MaxLatency)
	}
	return
}

func (w *FlushingWriter) delayedFlush() {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	if !w.flushPending {
		return
	}
	w.WriterFlusher.Flush()
	w.flushPending = false
}

func (w *FlushingWriter) flush() {
	if w.hijacked {
		return
	}
	w.flushPending = false
	if w.timer != nil {
		w.timer.Stop()
	}
	w.WriterFlusher.Flush()
}

func (w *FlushingWriter) Flush() {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	w.flush()
}

func (w *FlushingWriter) Close() {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	w.flush()
	w.closed = true
}

// Wrote returns whether the method WriteHeader has been called or not.
func (w *FlushingWriter) Wrote() bool {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	return w.wrote
}

// Hijack will hijack the underlying TCP connection, if available in the
// ResponseWriter.
func (w *FlushingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.WriterFlusher.(http.Hijacker); ok {
		w.hijacked = true
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("cannot hijack connection")
}
