// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"

	"gopkg.in/check.v1"
)

type MultiLoggerSuite struct {
	logger     Logger
	buf1, buf2 bytes.Buffer
}

var _ = check.Suite(&MultiLoggerSuite{})

func (s *MultiLoggerSuite) SetUpTest(c *check.C) {
	s.logger = NewMultiLogger(
		newWriterLogger(&s.buf1, true),
		newWriterLogger(&s.buf2, true),
	)
}

func (s *MultiLoggerSuite) TearDownTest(c *check.C) {
	s.buf1.Reset()
	s.buf2.Reset()
}

func (s *MultiLoggerSuite) TestDebug(c *check.C) {
	s.logger.Debug("something went wrong")
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*DEBUG: something went wrong$`)
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*DEBUG: something went wrong$`)
}

func (s *MultiLoggerSuite) TestDebugf(c *check.C) {
	s.logger.Debugf("something went wrong: %q", "this")
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*DEBUG: something went wrong: "this"$`)
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*DEBUG: something went wrong: "this"$`)
}

func (s *MultiLoggerSuite) TestError(c *check.C) {
	s.logger.Error("something went wrong")
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*ERROR: something went wrong$`)
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*ERROR: something went wrong$`)
}

func (s *MultiLoggerSuite) TestErrorf(c *check.C) {
	s.logger.Errorf("something went wrong: %q", "this")
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*ERROR: something went wrong: "this"$`)
	c.Check(s.buf1.String(), check.Matches, `(?m)^.*ERROR: something went wrong: "this"$`)
}
