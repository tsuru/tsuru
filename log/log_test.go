// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"launchpad.net/gocheck"
	"log"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestLogPanic(c *gocheck.C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	defer func() {
		c.Assert(recover(), gocheck.Equals, "log anything")
	}()
	Panic("log anything")
}

func (s *S) TestLogPanicf(c *gocheck.C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	defer func() {
		c.Assert(recover(), gocheck.Equals, "log anything formatted")
	}()
	Panicf("log anything %s", "formatted")
}

func (s *S) TestLogPrint(c *gocheck.C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	Print("log anything")
	c.Assert(buf.String(), gocheck.Equals, "log anything\n")
}

func (s *S) TestLogPrintf(c *gocheck.C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	SetLogger(log.New(buf, "", 0))
	Printf("log anything %d", 1)
	c.Assert(buf.String(), gocheck.Equals, "log anything 1\n")
}

func (s *S) TestLogFatalWithoutTarget(c *gocheck.C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Fatal("log anything")
}

func (s *S) TestLogPanicWithoutTarget(c *gocheck.C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Panic("log anything")
}

func (s *S) TestLogPrintWithoutTarget(c *gocheck.C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Print("log anything")
}

func (s *S) TestLogPrintfWithoutTarget(c *gocheck.C) {
	SetLogger(nil)
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Printf("log anything %d", 1)
}

func (s *S) TestWrite(c *gocheck.C) {
	w := &bytes.Buffer{}
	err := Write(w, []byte("teeest"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(w.String(), gocheck.Equals, "teeest")
}

func BenchmarkLogging(b *testing.B) {
	var buf bytes.Buffer
	target := new(Target)
	target.SetLogger(log.New(&buf, "", 0))
	for i := 0; i < b.N; i++ {
		target.Printf("Log message number %d.", i+1)
	}
}
