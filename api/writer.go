// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/globocom/tsuru/app"
	"io"
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
