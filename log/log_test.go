// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"log"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func newFakeLogger() *bytes.Buffer {
	l := NewFileLogger("/dev/null", true)
	fl, _ := l.(*fileLogger)
	b := &bytes.Buffer{}
	fl.logger = log.New(b, "", 0)
	SetLogger(l)
	return b
}

func (s *S) TestLogError(c *check.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Error("log anything")
	c.Assert(buf.String(), check.Equals, "ERROR: log anything\n")
}

func (s *S) TestLogErrorf(c *check.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Errorf("log anything %d", 1)
	c.Assert(buf.String(), check.Equals, "ERROR: log anything 1\n")
}

func (s *S) TestLogErrorWithoutTarget(c *check.C) {
	_ = newFakeLogger()
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	Error("log anything")
}

func (s *S) TestLogErrorfWithoutTarget(c *check.C) {
	_ = newFakeLogger()
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	Errorf("log anything %d", 1)
}

func (s *S) TestLogDebug(c *check.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Debug("log anything")
	c.Assert(buf.String(), check.Equals, "DEBUG: log anything\n")
}

func (s *S) TestLogDebugf(c *check.C) {
	buf := newFakeLogger()
	defer buf.Reset()
	Debugf("log anything %d", 1)
	c.Assert(buf.String(), check.Equals, "DEBUG: log anything 1\n")
}

func (s *S) TestWrite(c *check.C) {
	w := &bytes.Buffer{}
	err := Write(w, []byte("teeest"))
	c.Assert(err, check.IsNil)
	c.Assert(w.String(), check.Equals, "teeest")
}
