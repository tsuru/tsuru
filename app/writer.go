// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
    "io"
)

type LogWriter struct {
	App    *App
	Writer io.Writer
}

// Write writes and logs the data.
func (w *LogWriter) Write(data []byte) (int, error) {
	err := w.App.Log(string(data), "tsuru")
	if err != nil {
		return 0, err
	}
	return w.Writer.Write(data)
}
