// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"testing"

	tTesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	stdin   *os.File
	recover []string
}

var _ = gocheck.Suite(&S{})
var manager *Manager

func (s *S) SetUpSuite(c *gocheck.C) {
	s.recover = tTesting.SetTargetFile(c, []byte("http://localhost"))
}

func (s *S) TearDownSuite(c *gocheck.C) {
	tTesting.RollbackFile(s.recover)
}

func (s *S) SetUpTest(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	manager.e = &exiter
}
