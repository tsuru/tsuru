// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"bytes"
	"launchpad.net/gocheck"
	"log"
)

type FileLoggerSuite struct {
	l  Logger
	fl *fileLogger
	b  *bytes.Buffer
}

var _ = gocheck.Suite(&FileLoggerSuite{})

func (s *FileLoggerSuite) SetUpSuite(c *gocheck.C) {
	s.l = newFileLogger("/dev/null")
	s.fl, _ = s.l.(*fileLogger)
}

func (s *FileLoggerSuite) SetUpTest(c *gocheck.C) {
	s.b = &bytes.Buffer{}
	s.fl.logger = log.New(s.b, "", log.LstdFlags)
}

func (s *FileLoggerSuite) TestNewFileLoggerReturnsALogger(c *gocheck.C) {
	_, ok := s.l.(Logger)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *FileLoggerSuite) TestNewFileLoggerInitializesWriter(c *gocheck.C) {
	c.Assert(s.fl.logger, gocheck.FitsTypeOf, &log.Logger{})
}

func (s *FileLoggerSuite) TestErrorShouldPrefixMessage(c *gocheck.C) {
	s.l.Error("something terrible happened")
	c.Assert(s.b.String(), gocheck.Matches, ".* ERROR: something terrible happened\n$")
}

func (s *FileLoggerSuite) TestErrorfShouldFormatErrorAndPrefixMessage(c *gocheck.C) {
	s.l.Errorf(`this is the error: "%s"`, "something bad happened")
	c.Assert(s.b.String(), gocheck.Matches, `.* ERROR: this is the error: "something bad happened"\n$`)
}

func (s *FileLoggerSuite) TestDebugShouldPrefixMessage(c *gocheck.C) {
	s.l.Debug("doing some stuff here")
	c.Assert(s.b.String(), gocheck.Matches, ".* DEBUG: doing some stuff here\n$")
}

func (s *FileLoggerSuite) TestDebugfShouldFormatAndPrefixMessage(c *gocheck.C) {
	s.l.Debugf(`message is "%s"`, "some debug message")
	c.Assert(s.b.String(), gocheck.Matches, `.* DEBUG: message is "some debug message"\n$`)
}
