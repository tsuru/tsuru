// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exec

import (
	"bytes"
	"testing"

	"github.com/tsuru/commandmocker"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) TestOsExecutorImplementsExecutor(c *check.C) {
	var _ Executor = OsExecutor{}
}

func (s *S) TestExecute(c *check.C) {
	tmpdir, err := commandmocker.Add("ls", "ok")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	var e OsExecutor
	var b bytes.Buffer
	opts := ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), check.Equals, true)
	expected := []string{"-lsa"}
	c.Assert(commandmocker.Parameters(tmpdir), check.DeepEquals, expected)
	c.Assert(b.String(), check.Equals, "ok")
}

func (s *S) TestExecuteWithoutArgs(c *check.C) {
	tmpdir, err := commandmocker.Add("ls", "ok")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	var e OsExecutor
	var b bytes.Buffer
	opts := ExecuteOptions{
		Cmd:    "ls",
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), check.Equals, true)
	c.Assert(commandmocker.Parameters(tmpdir), check.IsNil)
	c.Assert(b.String(), check.Equals, "ok")
}
