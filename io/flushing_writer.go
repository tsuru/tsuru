// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"net/http"
	"sync"
)

var writeMutex sync.Mutex

// FlushingWriter is a custom writer that flushes after writing, if the
// underlying ResponseWriter is also an http.Flusher.
type FlushingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *FlushingWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

// Write writes and flushes the data.
func (w *FlushingWriter) Write(data []byte) (int, error) {
	writeMutex.Lock()
	defer writeMutex.Unlock()
	w.wrote = true
	n, err := w.ResponseWriter.Write(data)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

// Wrote returns whether the method WriteHeader has been called or not.
func (w *FlushingWriter) Wrote() bool {
	return w.wrote
}
