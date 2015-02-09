// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"log"
	"log/syslog"

	"gopkg.in/check.v1"
)

type SyslogLoggerSuite struct {
	l  Logger
	sl *syslogLogger
}

var _ = check.Suite(&SyslogLoggerSuite{})

func (s *SyslogLoggerSuite) SetUpSuite(c *check.C) {
	s.l = NewSyslogLogger("tsr", true)
	s.sl = s.l.(*syslogLogger)
}

func (s *SyslogLoggerSuite) TestNewSyslogLoggerReturnsALogger(c *check.C) {
	_, ok := s.l.(Logger)
	c.Assert(ok, check.Equals, true)
}

func (s *SyslogLoggerSuite) TestNewSyslogLoggerInstantiatesSyslogWriter(c *check.C) {
	c.Assert(s.sl.w, check.FitsTypeOf, &syslog.Writer{})
}

func (s *SyslogLoggerSuite) TestGetStdLoggerShouldReturnValidLogger(c *check.C) {
	c.Assert(s.sl.GetStdLogger(), check.FitsTypeOf, &log.Logger{})
}
