// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"
	"io"
	"sync"
)

// Reader is a thread safe version of bytes.Reader.
type Reader struct {
	reader bytes.Reader
	mutex  sync.Mutex
}

func NewReader(b []byte) *Reader {
	reader := bytes.NewReader(b)
	return &Reader{reader: *reader}
}

func (r *Reader) Len() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Len()
}

func (r *Reader) Read(b []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Read(b)
}

func (r *Reader) ReadAt(b []byte, off int64) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadAt(b, off)
}

func (r *Reader) ReadByte() (byte, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadByte()
}

func (r *Reader) ReadRune() (rune, int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadRune()
}

func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Seek(offset, whence)
}

func (r *Reader) UnreadByte() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.UnreadByte()
}

func (r *Reader) UnreadRune() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.UnreadRune()
}

func (r *Reader) WriteTo(w io.Writer) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.WriteTo(w)
}
