// Copyright 2014 tsuru authors. All rights reserved.
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

func newFakeLogger() *bytes.Buffer {
	l := NewFileLogger("/dev/null", true)
	fl, _ := l.(*fileLogger)
	b := &bytes.Buffer{}
	fl.logger = log.New(b, "", 0)
	SetLogger(l)
	return b
}

func (s *S) TestLogError(c *gocheck.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Error("log anything")
	c.Assert(buf.String(), gocheck.Equals, "ERROR: log anything\n")
}

func (s *S) TestLogErrorf(c *gocheck.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Errorf("log anything %d", 1)
	c.Assert(buf.String(), gocheck.Equals, "ERROR: log anything 1\n")
}

func (s *S) TestLogErrorWithoutTarget(c *gocheck.C) {
	_ = newFakeLogger()
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Error("log anything")
}

func (s *S) TestLogErrorfWithoutTarget(c *gocheck.C) {
	_ = newFakeLogger()
	defer func() {
		c.Assert(recover(), gocheck.IsNil)
	}()
	Errorf("log anything %d", 1)
}

func (s *S) TestLogDebug(c *gocheck.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Debug("log anything")
	c.Assert(buf.String(), gocheck.Equals, "DEBUG: log anything\n")
}

func (s *S) TestLogDebugf(c *gocheck.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Debugf("log anything %d", 1)
	c.Assert(buf.String(), gocheck.Equals, "DEBUG: log anything 1\n")
}

func (s *S) TestWrite(c *gocheck.C) {
	w := &bytes.Buffer{}
	err := Write(w, []byte("teeest"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(w.String(), gocheck.Equals, "teeest")
}
