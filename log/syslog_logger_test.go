// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"launchpad.net/gocheck"
	"log"
	"log/syslog"
)

type SyslogLoggerSuite struct {
	l  Logger
	sl *syslogLogger
}

var _ = gocheck.Suite(&SyslogLoggerSuite{})

func (s *SyslogLoggerSuite) SetUpSuite(c *gocheck.C) {
	s.l = NewSyslogLogger("tsr", true)
	s.sl = s.l.(*syslogLogger)
}

func (s *SyslogLoggerSuite) TestNewSyslogLoggerReturnsALogger(c *gocheck.C) {
	_, ok := s.l.(Logger)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *SyslogLoggerSuite) TestNewSyslogLoggerInstantiatesSyslogWriter(c *gocheck.C) {
	c.Assert(s.sl.w, gocheck.FitsTypeOf, &syslog.Writer{})
}

func (s *SyslogLoggerSuite) TestGetStdLoggerShouldReturnValidLogger(c *gocheck.C) {
	c.Assert(s.sl.GetStdLogger(), gocheck.FitsTypeOf, &log.Logger{})
}
