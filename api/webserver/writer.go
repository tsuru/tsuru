// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/api/filter"
	"net/http"
)

// FilteredWriter is a custom writer
// that filter deprecation warnings and juju log output.
type FilteredWriter struct {
	writer http.ResponseWriter
}

// WriteHeader calls the w.Header
func (w *FilteredWriter) Header() http.Header {
	return w.writer.Header()
}

// Write writes and flushes the data, filtering the juju warnings.
func (w *FilteredWriter) Write(data []byte) (int, error) {
	_, err := w.writer.Write(filter.FilterOutput(data))
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
	// returning the len(data) to skip the 'short write' error
	return len(data), err
}

// WriteHeader calls the w.WriteHeader
func (w *FilteredWriter) WriteHeader(code int) {
	w.writer.WriteHeader(code)
}
