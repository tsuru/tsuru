// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "io"

type Logger interface {
	Log(string, string, string) error
}

type LogWriter struct {
	App    Logger
	Writer io.Writer
	Source string
}

// Write writes and logs the data.
func (w *LogWriter) Write(data []byte) (int, error) {
	source := w.Source
	if source == "" {
		source = "tsuru"
	}
	err := w.App.Log(string(data), source, "api")
	if err != nil {
		return 0, err
	}
	return w.Writer.Write(data)
}
