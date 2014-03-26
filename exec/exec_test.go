// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exec

import (
	"bytes"
	"github.com/tsuru/commandmocker"
	"launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) TestOsExecutorImplementsExecutor(c *gocheck.C) {
	var _ Executor = OsExecutor{}
}

func (s *S) TestExecute(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("ls", "ok")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	var e OsExecutor
	var b bytes.Buffer
	err = e.Execute("ls", []string{"-lsa"}, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := []string{"-lsa"}
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.DeepEquals, expected)
	c.Assert(b.String(), gocheck.Equals, "ok")
}

func (s *S) TestExecuteWithoutArgs(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("ls", "ok")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	var e OsExecutor
	var b bytes.Buffer
	err = e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	c.Assert(commandmocker.Parameters(tmpdir), gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "ok")
}
