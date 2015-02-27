// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/tsuru/tsuru/log"
)

// FlushingWriter is a custom writer that flushes after writing, if the
// underlying ResponseWriter is also an http.Flusher.
type FlushingWriter struct {
	http.ResponseWriter
	wrote      bool
	writeMutex sync.Mutex
}

func (w *FlushingWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

// Write writes and flushes the data.
func (w *FlushingWriter) Write(data []byte) (written int, err error) {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	w.wrote = true
	written, err = w.ResponseWriter.Write(data)
	if err != nil {
		return
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("Error recovered on flushing writer: %#v", r)
				log.Debugf(msg)
				err = fmt.Errorf(msg)
			}
		}()
		f.Flush()
	}
	return
}

// Wrote returns whether the method WriteHeader has been called or not.
func (w *FlushingWriter) Wrote() bool {
	return w.wrote
}

// Hijack will hijack the underlying TCP connection, if available in the
// ResponseWriter.
func (w *FlushingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("cannot hijack connection")
}
