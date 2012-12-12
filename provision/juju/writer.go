// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"net/http"
)

// Writer is a custom writer that filters output from Juju.
//
// It ignores all Juju logging and Python warnings.
type Writer struct {
	http.ResponseWriter
	wrote bool
}

func (w *Writer) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

// Write writes data to the underlying writer, filtering the juju warnings. If
// the underlying Writer implements http.Flusher, it will also the Flush
// method.
func (w *Writer) Write(data []byte) (int, error) {
	w.wrote = true
	originalLength := len(data)
	if w.Header().Get("Content-Type") == "text" {
		data = FilterOutput(data)
	}
	_, err := w.ResponseWriter.Write(data)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	// returning the len(data) to skip the "short write" error
	return originalLength, err
}
