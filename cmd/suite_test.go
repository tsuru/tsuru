// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	stdin        *os.File
	recover      []string
	recoverToken []string
}

var _ = gocheck.Suite(&S{})
var manager *Manager

func (s *S) SetUpSuite(c *gocheck.C) {
	s.recover = cmdtest.SetTargetFile(c, []byte("http://localhost"))
	s.recoverToken = cmdtest.SetTokenFile(c, []byte("abc123"))
}

func (s *S) TearDownSuite(c *gocheck.C) {
	cmdtest.RollbackFile(s.recover)
	cmdtest.RollbackFile(s.recoverToken)
}

func (s *S) SetUpTest(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	manager.e = &exiter
}
