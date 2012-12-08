// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/juju"
	"net/http"
)

// FilteredWriter is a custom writer
// that filter deprecation warnings and juju log output.
type FilteredWriter struct {
	http.ResponseWriter
	wrote bool
}

// Write writes and flushes the data, filtering the juju warnings.
func (w *FilteredWriter) Write(data []byte) (int, error) {
	w.wrote = true
	originalLength := len(data)
	if w.Header().Get("Content-Type") == "text" {
		data = juju.FilterOutput(data)
	}
	_, err := w.ResponseWriter.Write(data)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	// returning the len(data) to skip the "short write" error
	return originalLength, err
}
