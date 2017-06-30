// Copyright 2015 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"bytes"
	"errors"

	"gopkg.in/check.v1"
)

type CheckerSuite struct{}

var _ = check.Suite(&CheckerSuite{})

func (s *CheckerSuite) TestCheckExecuteCheckerFunc(c *check.C) {
	err := Check([]Checker{
		func() error { return errors.New("Fake checker error") },
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Fake checker error")
}

func (s *CheckerSuite) TestCheckConsideringWarningsAsErrors(c *check.C) {
	err := Check([]Checker{
		func() error { return NewWarning("my warning") },
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "my warning")
}

func (s *CheckerSuite) TestCheckWithWarnings(c *check.C) {
	buf := bytes.NewBuffer(nil)
	err := CheckWithWarnings([]Checker{
		func() error { return NewWarning("my warning") },
	}, buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "WARNING: my warning\n")
}
