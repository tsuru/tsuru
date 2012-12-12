// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"io"
	"net/http"
)

// Writer is a custom writer that filters output from Juju.
//
// It ignores all Juju logging and Python warnings.
type Writer struct {
	io.Writer
}

// Write writes data to the underlying writer, filtering the juju warnings.
func (w *Writer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if rw, ok := w.Writer.(http.ResponseWriter); ok {
		if rw.Header().Get("Content-Type") == "text" {
			data = FilterOutput(data)
		}
	} else {
		data = FilterOutput(data)
	}
	_, err := w.Writer.Write(data)
	// returning the len(data) to skip the "short write" error
	return originalLength, err
}
