// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"net/http"
)

// FlushingWriter is a custom writer that flushes after writing, if the
// underlying ResponseWriter is also an http.Flusher.
type flushingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *flushingWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

// Write writes and flushes the data.
func (w *flushingWriter) Write(data []byte) (int, error) {
	w.wrote = true
	n, err := w.ResponseWriter.Write(data)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}
