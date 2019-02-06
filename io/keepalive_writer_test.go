// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"errors"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	check "gopkg.in/check.v1"
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

func (s *S) TestKeepAliveWriter(c *check.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "...")
	defer w.Stop()
	w.writeLock.Lock()
	w.testCh = make(chan struct{})
	w.writeLock.Unlock()
	<-w.testCh
	c.Check(buf.String(), check.Equals, "\n...\n")
	count, err := w.Write([]byte("xxx"))
	c.Check(err, check.IsNil)
	c.Check(count, check.Equals, 3)
	c.Check(buf.String(), check.Equals, "\n...\nxxx")
	<-w.testCh
	c.Check(buf.String(), check.Equals, "\n...\nxxx\n...\n")
	buf.Close()
	<-w.testCh
	c.Check(buf.String(), check.Equals, "\n...\nxxx\n...\n")
	c.Check(buf.callCount, check.Equals, 4)
}

func (s *S) TestKeepAliveWriterDoesntWriteMultipleNewlines(c *check.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "---")
	defer w.Stop()
	w.writeLock.Lock()
	w.testCh = make(chan struct{})
	w.writeLock.Unlock()
	count, err := w.Write([]byte("xxx\n"))
	c.Check(err, check.IsNil)
	c.Check(count, check.Equals, 4)
	<-w.testCh
	c.Check(buf.String(), check.Equals, "xxx\n---\n")
	<-w.testCh
	c.Check(buf.String(), check.Equals, "xxx\n---\n---\n")
}

func (s *S) TestKeepAliveWriterEmptyContent(c *check.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "---")
	close(w.ping)
	count, err := w.Write(nil)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestKeepAliveWriterAfterError(c *check.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "...")
	count, err := w.Write([]byte("xxx"))
	c.Check(err, check.IsNil)
	c.Check(count, check.Equals, 3)
	buf.Close()
	count, err = w.Write([]byte("111"))
	c.Check(err, check.ErrorMatches, "Closed error.")
	c.Check(count, check.Equals, 0)
	count, err = w.Write([]byte("222"))
	c.Check(err, check.ErrorMatches, "Closed error.")
	c.Check(count, check.Equals, 0)
}

func (s *S) TestKeepAliveWriterRace(c *check.C) {
	var buf closableBuffer
	w := NewKeepAliveWriter(&buf, 100*time.Millisecond, "...")
	nWrites := 10
	wg := sync.WaitGroup{}
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < nWrites; i++ {
				count, err := w.Write([]byte("xxx"))
				c.Check(err, check.IsNil)
				c.Check(count, check.Equals, 3)
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		pprof.Lookup("goroutine").WriteTo(os.Stderr, 2)
		c.Fatalf("timeout after 15s waiting for test to finish")
	}
	buf.Close()
	c.Assert(strings.Count(buf.String(), "xxx"), check.Equals, nWrites*2)
	c.Assert(strings.Contains(buf.String(), "..."), check.Equals, true)
	c.Assert(strings.Contains(buf.String(), "......"), check.Equals, false)
}
