// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"
	"io"
	"sync"
)

// BytesReader is a thread safe version of bytes.Reader.
type BytesReader struct {
	reader bytes.Reader
	mutex  sync.Mutex
}

func NewBytesReader(b []byte) *BytesReader {
	reader := bytes.NewReader(b)
	return &BytesReader{reader: *reader}
}

func (r *BytesReader) Len() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Len()
}

func (r *BytesReader) Read(b []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Read(b)
}

func (r *BytesReader) ReadAt(b []byte, off int64) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadAt(b, off)
}

func (r *BytesReader) ReadByte() (byte, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadByte()
}

func (r *BytesReader) ReadRune() (rune, int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.ReadRune()
}

func (r *BytesReader) Seek(offset int64, whence int) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.Seek(offset, whence)
}

func (r *BytesReader) UnreadByte() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.UnreadByte()
}

func (r *BytesReader) UnreadRune() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.UnreadRune()
}

func (r *BytesReader) WriteTo(w io.Writer) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.reader.WriteTo(w)
}
