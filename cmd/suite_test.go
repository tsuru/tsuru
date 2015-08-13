// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"testing"

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

func (s *S) SetUpTest(c *check.C) {
	var stdout, stderr bytes.Buffer
	manager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	manager.e = &exiter
	os.Setenv("TSURU_TARGET", "http://localhost")
	os.Setenv("TSURU_TOKEN", "abc123")
	if env := os.Getenv("TERM"); env == "" {
		os.Setenv("TERM", "tsuruterm")
	}
}

func (s *S) TearDownTest(c *check.C) {
	os.Unsetenv("TSURU_TARGET")
	os.Unsetenv("TSURU_TOKEN")
}
