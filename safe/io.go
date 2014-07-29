// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"io"
	"sync"
)

type Writer struct {
	w   io.Writer
	mut sync.Mutex
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.w.Write(p)
}

type Reader struct {
	r   io.Reader
	mut sync.Mutex
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read(p []byte) (int, error) {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.r.Read(p)
}
