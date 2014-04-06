// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package testing

import (
	"github.com/tsuru/tsuru/log"
	"launchpad.net/gocheck"
	"testing"
)

type FakeLoggerSuite struct {
	l  log.Logger
	fl *FakeLogger
}

var _ = gocheck.Suite(&FakeLoggerSuite{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *FakeLoggerSuite) SetUpSuite(c *gocheck.C) {
	s.l = NewFakeLogger()
	s.fl = s.l.(*FakeLogger)
}

func (s *FakeLoggerSuite) TearDownTest(c *gocheck.C) {
	s.fl.Buf.Reset()
}

func (s *FakeLoggerSuite) TestNewFakeLoggerImplementsLoggerInterface(c *gocheck.C) {
	_, ok := s.l.(log.Logger)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *FakeLoggerSuite) TestErrorWritesOnBuffer(c *gocheck.C) {
	s.l.Error("some error")
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some error\n")
}

func (s *FakeLoggerSuite) TestErrorfWritesOnBuffer(c *gocheck.C) {
	s.l.Errorf("some error %d", 1)
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some error 1\n")
}

func (s *FakeLoggerSuite) TestDebugWritesOnBuffer(c *gocheck.C) {
	s.l.Debug("some info")
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some info\n")
}

func (s *FakeLoggerSuite) TestDebugfWritesOnBuffer(c *gocheck.C) {
	s.l.Debugf("some info %d", 1)
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some info 1\n")
}

func (s *FakeLoggerSuite) TestFatalWritesOnBuffer(c *gocheck.C) {
	s.l.Fatal("some fatal error")
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some fatal error\n")
}

func (s *FakeLoggerSuite) TestFatalfWritesOnBuffer(c *gocheck.C) {
	s.l.Fatalf("some fatal error %d", 1)
	c.Assert(s.fl.Buf.String(), gocheck.Equals, "some fatal error 1\n")
}
