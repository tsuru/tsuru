// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"io"
	"sync"
)

// SafeBuffer is a thread safe version of bytes.Buffer.
type SafeBuffer struct {
	buf bytes.Buffer
	mut sync.Mutex
}

func NewSafeBuffer(b []byte) *SafeBuffer {
	buf := bytes.NewBuffer(b)
	return &SafeBuffer{buf: *buf}
}

func (sb *SafeBuffer) Bytes() []byte {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Bytes()
}

func (sb *SafeBuffer) Len() int {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Len()
}

func (sb *SafeBuffer) Next(n int) []byte {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Next(n)
}

func (sb *SafeBuffer) Read(p []byte) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) ReadByte() (byte, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadByte()
}

func (sb *SafeBuffer) ReadBytes(delim byte) ([]byte, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadBytes(delim)
}

func (sb *SafeBuffer) ReadFrom(r io.Reader) (int64, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadFrom(r)
}

func (sb *SafeBuffer) ReadRune() (rune, int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadRune()
}

func (sb *SafeBuffer) ReadString(delim byte) (string, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadString(delim)
}

func (sb *SafeBuffer) Reset() {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	sb.buf.Reset()
}

func (sb *SafeBuffer) String() string {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.String()
}

func (sb *SafeBuffer) Truncate(n int) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	sb.buf.Truncate(n)
}

func (sb *SafeBuffer) UnreadByte() error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.UnreadByte()
}

func (sb *SafeBuffer) UnreadRune() error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.UnreadRune()
}

func (sb *SafeBuffer) Write(p []byte) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Write(p)
}

func (sb *SafeBuffer) WriteByte(c byte) error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteByte(c)
}

func (sb *SafeBuffer) WriteRune(r rune) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteRune(r)
}

func (sb *SafeBuffer) WriteTo(w io.Writer) (int64, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteTo(w)
}
