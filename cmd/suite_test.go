// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	stdin        *os.File
	recover      []string
	recoverToken []string
}

var _ = check.Suite(&S{})
var manager *Manager

func (s *S) SetUpSuite(c *check.C) {
	s.recover = cmdtest.SetTargetFile(c, []byte("http://localhost"))
	s.recoverToken = cmdtest.SetTokenFile(c, []byte("abc123"))
	if env := os.Getenv("TERM"); env == "" {
		os.Setenv("TERM", "tsuruterm")
	}
}

func (s *S) TearDownSuite(c *check.C) {
	cmdtest.RollbackFile(s.recover)
	cmdtest.RollbackFile(s.recoverToken)
}

func (s *S) SetUpTest(c *check.C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	manager.e = &exiter
}
