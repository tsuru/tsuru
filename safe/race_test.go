// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package safe

import (
	"bytes"
	"sync"

	"launchpad.net/gocheck"
)

func (s *S) TestSafeBufferIsThreadSafe(c *gocheck.C) {
	var buf Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		buf.Write([]byte("something"))
		wg.Done()
	}()
	go func() {
		var p [4]byte
		buf.Read(p[:])
		wg.Done()
	}()
	buf.Reset()
	wg.Wait()
}

func (s *S) TestSafeWriterIsThreadSafe(c *gocheck.C) {
	var buf bytes.Buffer
	writer := NewWriter(&buf)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		writer.Write([]byte("something"))
		wg.Done()
	}()
	go func() {
		writer.Write([]byte("otherthing"))
		wg.Done()
	}()
	wg.Wait()
}

func (s *S) TestSafeReaderIsThreadSafe(c *gocheck.C) {
	var buf [8]byte
	buffer := bytes.NewBufferString("hello world something")
	reader := NewReader(buffer)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		reader.Read(buf[:])
		wg.Done()
	}()
	go func() {
		reader.Read(buf[:])
		wg.Done()
	}()
	wg.Wait()
}
