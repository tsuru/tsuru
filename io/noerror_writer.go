// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"io"
	"sync/atomic"
)

type NoErrorWriter struct {
	io.Writer
	withError int32
}

func (w *NoErrorWriter) Write(data []byte) (int, error) {
	if atomic.LoadInt32(&w.withError) == 1 {
		return len(data), nil
	}
	n, err := w.Writer.Write(data)
	if err != nil || n != len(data) {
		atomic.StoreInt32(&w.withError, 1)
	}
	return len(data), nil
}
