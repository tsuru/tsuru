// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/app"
	"io"
	"net/http"
)

type LogWriter struct {
	app    *app.App
	writer io.Writer
}

// Write writes and logs the data.
func (w *LogWriter) Write(data []byte) (int, error) {
	err := w.app.Log(string(data), "tsuru")
	if err != nil {
		return 0, err
	}
	return w.writer.Write(data)
}

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
	w.wrote = true
	n, err := w.ResponseWriter.Write(data)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}
