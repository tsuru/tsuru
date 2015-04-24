// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"log"

	"gopkg.in/check.v1"
)

type FileLoggerSuite struct {
	l  Logger
	fl *fileLogger
	b  *bytes.Buffer
}

var _ = check.Suite(&FileLoggerSuite{})

func (s *FileLoggerSuite) SetUpSuite(c *check.C) {
	s.l = NewFileLogger("/dev/null", true)
	s.fl, _ = s.l.(*fileLogger)
}

func (s *FileLoggerSuite) SetUpTest(c *check.C) {
	s.b = &bytes.Buffer{}
	s.fl.logger = log.New(s.b, "", log.LstdFlags)
}

func (s *FileLoggerSuite) TestNewFileLoggerReturnsALogger(c *check.C) {
	_, ok := s.l.(Logger)
	c.Assert(ok, check.Equals, true)
}

func (s *FileLoggerSuite) TestNewWriterLogger(c *check.C) {
	var buf bytes.Buffer
	logger := NewWriterLogger(&buf, true)
	logger.Errorf("something went wrong: %s", "this")
	c.Assert(buf.String(), check.Matches, `(?m)^.*ERROR: something went wrong: this$`)
}

func (s *FileLoggerSuite) TestNewFileLoggerInitializesWriter(c *check.C) {
	c.Assert(s.fl.logger, check.FitsTypeOf, &log.Logger{})
}

func (s *FileLoggerSuite) TestErrorShouldPrefixMessage(c *check.C) {
	s.l.Error("something terrible happened")
	c.Assert(s.b.String(), check.Matches, ".* ERROR: something terrible happened\n$")
}

func (s *FileLoggerSuite) TestErrorfShouldFormatErrorAndPrefixMessage(c *check.C) {
	s.l.Errorf(`this is the error: "%s"`, "something bad happened")
	c.Assert(s.b.String(), check.Matches, `.* ERROR: this is the error: "something bad happened"\n$`)
}

func (s *FileLoggerSuite) TestDebugShouldPrefixMessage(c *check.C) {
	s.l.Debug("doing some stuff here")
	c.Assert(s.b.String(), check.Matches, ".* DEBUG: doing some stuff here\n$")
}

func (s *FileLoggerSuite) TestDebugfShouldFormatAndPrefixMessage(c *check.C) {
	s.l.Debugf(`message is "%s"`, "some debug message")
	c.Assert(s.b.String(), check.Matches, `.* DEBUG: message is "some debug message"\n$`)
}

func (s *FileLoggerSuite) TestDebugShouldNotWriteDebugIsSetToFalse(c *check.C) {
	l := NewFileLogger("/dev/null", false)
	fl, _ := l.(*fileLogger)
	b := &bytes.Buffer{}
	fl.logger = log.New(b, "", log.LstdFlags)
	l.Debug("sould not log this")
	c.Assert(b.String(), check.Equals, "")
	l.Debugf("sould not log this either %d", 1)
	c.Assert(b.String(), check.Equals, "")
}

func (s *FileLoggerSuite) TestErrorShouldWriteWhenDebugIsFalse(c *check.C) {
	l := NewFileLogger("/dev/null", false)
	fl, _ := l.(*fileLogger)
	b := &bytes.Buffer{}
	fl.logger = log.New(b, "", log.LstdFlags)
	l.Error("should write this")
	c.Assert(b.String(), check.Matches, `.* ERROR: should write this\n$`)
}

func (s *FileLoggerSuite) TestGetStdLoggerShouldReturnValidLogger(c *check.C) {
	logger := s.l.GetStdLogger()
	logger.Printf(`message is "%s"`, "some debug message")
	c.Assert(s.b.String(), check.Matches, `.*message is "some debug message"\n$`)
}
