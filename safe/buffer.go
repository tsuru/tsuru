// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package safe provides some thread safe types, wrapping builtin types.
package safe

import (
	"bytes"
	"io"
	"sync"
)

// Buffer is a thread safe version of bytes.Buffer.
type Buffer struct {
	buf bytes.Buffer
	mut sync.Mutex
}

func NewBuffer(b []byte) *Buffer {
	buf := bytes.NewBuffer(b)
	return &Buffer{buf: *buf}
}

func (sb *Buffer) Bytes() []byte {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Bytes()
}

func (sb *Buffer) Len() int {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Len()
}

func (sb *Buffer) Next(n int) []byte {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Next(n)
}

func (sb *Buffer) Read(p []byte) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Read(p)
}

func (sb *Buffer) ReadByte() (byte, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadByte()
}

func (sb *Buffer) ReadBytes(delim byte) ([]byte, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadBytes(delim)
}

func (sb *Buffer) ReadFrom(r io.Reader) (int64, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadFrom(r)
}

func (sb *Buffer) ReadRune() (rune, int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadRune()
}

func (sb *Buffer) ReadString(delim byte) (string, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.ReadString(delim)
}

func (sb *Buffer) Reset() {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	sb.buf.Reset()
}

func (sb *Buffer) String() string {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.String()
}

func (sb *Buffer) Truncate(n int) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	sb.buf.Truncate(n)
}

func (sb *Buffer) UnreadByte() error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.UnreadByte()
}

func (sb *Buffer) UnreadRune() error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.UnreadRune()
}

func (sb *Buffer) Write(p []byte) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Write(p)
}

func (sb *Buffer) WriteByte(c byte) error {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteByte(c)
}

func (sb *Buffer) WriteString(s string) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteString(s)
}

func (sb *Buffer) WriteRune(r rune) (int, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteRune(r)
}

func (sb *Buffer) WriteTo(w io.Writer) (int64, error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.WriteTo(w)
}
