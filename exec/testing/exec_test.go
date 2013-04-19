// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"github.com/globocom/tsuru/exec"
	"launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) TestFakeExecutorImplementsExecutor(c *gocheck.C) {
	var _ exec.Executor = &FakeExecutor{}
}

func (s *S) TestExecute(c *gocheck.C) {
	var e FakeExecutor
	var b bytes.Buffer
	cmd := "ls"
	args := []string{"-lsa"}
	err := e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(e.ExecutedCmd(cmd, args), gocheck.Equals, true)
}
