// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/tsuru/tsuru/cmd"
	tTesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
	"os"
	"testing"
)

type S struct {
	recover []string
}

func (s *S) SetUpSuite(c *gocheck.C) {
	s.recover = tTesting.SetTargetFile(c)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	tTesting.RollbackTargetFile(s.recover)
}

var _ = gocheck.Suite(&S{})
var manager *cmd.Manager

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) SetUpTest(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = cmd.NewManager("glb", version, header, &stdout, &stderr, os.Stdin, nil)
}
