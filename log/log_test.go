// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	. "launchpad.net/gocheck"
	"log"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestLogPanic(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	defer func() {
		c.Assert(recover(), Equals, "log anything")
	}()
	Panic("log anything")
}

func (s *S) TestLogPanicf(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	defer func() {
		c.Assert(recover(), Equals, "log anything formatted")
	}()
	Panicf("log anything %s", "formatted")
}

func (s *S) TestLogPrint(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	Print("log anything")
	c.Assert(buf.String(), Equals, "log anything\n")
}

func (s *S) TestLogPrintf(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	Printf("log anything %d", 1)
	c.Assert(buf.String(), Equals, "log anything 1\n")
}

func (s *S) TestLogFatalWithoutTarget(c *C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Fatal("log anything")
}

func (s *S) TestLogPanicWithoutTarget(c *C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Panic("log anything")
}

func (s *S) TestLogPrintWithoutTarget(c *C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Print("log anything")
}

func (s *S) TestLogPrintfWithoutTarget(c *C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Printf("log anything %d", 1)
}

func BenchmarkLogging(b *testing.B) {
	var buf bytes.Buffer
	target := new(Target)
	target.SetLogger(log.New(&buf, "", 0))
	for i := 0; i < b.N; i++ {
		target.Printf("Log message number %d.", i+1)
	}
}
