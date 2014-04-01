// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/exec/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestExecutor(c *gocheck.C) {
	execut = &testing.FakeExecutor{}
	c.Assert(executor(), gocheck.DeepEquals, execut)
	execut = nil
	c.Assert(executor(), gocheck.DeepEquals, exec.OsExecutor{})
}
