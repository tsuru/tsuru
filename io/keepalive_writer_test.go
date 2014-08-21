// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"errors"
	"launchpad.net/gocheck"
	"sync"
	"time"
)

type closableBuffer struct {
	bytes.Buffer
	closed     bool
	callCount  int
	writerLock sync.Mutex
}

func (b *closableBuffer) Write(bytes []byte) (int, error) {
	b.writerLock.Lock()
	defer b.writerLock.Unlock()
	b.callCount++
	if b.closed {
		return 0, errors.New("Closed error.")
	}
	return b.Buffer.Write(bytes)
}

func (b *closableBuffer) Close() error {
	b.writerLock.Lock()
	defer b.writerLock.Unlock()
	b.closed = true
	return nil
}

func (b *closableBuffer) String() string {
	b.writerLock.Lock()
	defer b.writerLock.Unlock()
	return b.Buffer.String()
}

func (s *S) TestKeepAliveWriter(c *gocheck.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "...")
	time.Sleep(150 * time.Millisecond)
	c.Check(buf.String(), gocheck.Equals, "\n...\n")
	count, err := w.Write([]byte("xxx"))
	c.Check(err, gocheck.IsNil)
	c.Check(count, gocheck.Equals, 3)
	c.Check(buf.String(), gocheck.Equals, "\n...\nxxx")
	time.Sleep(150 * time.Millisecond)
	c.Check(buf.String(), gocheck.Equals, "\n...\nxxx\n...\n")
	buf.Close()
	time.Sleep(300 * time.Millisecond)
	c.Check(buf.String(), gocheck.Equals, "\n...\nxxx\n...\n")
	c.Check(buf.callCount, gocheck.Equals, 4)
}

func (s *S) TestKeepAliveWriterDoesntWriteMultipleNewlines(c *gocheck.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "---")
	count, err := w.Write([]byte("xxx\n"))
	c.Check(err, gocheck.IsNil)
	c.Check(count, gocheck.Equals, 4)
	time.Sleep(120 * time.Millisecond)
	c.Check(buf.String(), gocheck.Equals, "xxx\n---\n")
	time.Sleep(120 * time.Millisecond)
	c.Check(buf.String(), gocheck.Equals, "xxx\n---\n---\n")
}

func (s *S) TestKeepAliveWriterEmptyContent(c *gocheck.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "---")
	close(w.ping)
	count, err := w.Write(nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *S) TestKeepAliveWriterAfterError(c *gocheck.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "...")
	count, err := w.Write([]byte("xxx"))
	c.Check(err, gocheck.IsNil)
	c.Check(count, gocheck.Equals, 3)
	buf.Close()
	count, err = w.Write([]byte("111"))
	c.Check(err, gocheck.ErrorMatches, "Closed error.")
	c.Check(count, gocheck.Equals, 0)
	count, err = w.Write([]byte("222"))
	c.Check(err, gocheck.ErrorMatches, "Closed error.")
	c.Check(count, gocheck.Equals, 0)
}
