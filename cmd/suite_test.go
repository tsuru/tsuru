// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})
var globalManager *Manager

func (s *S) SetUpTest(c *check.C) {
	var stdout, stderr bytes.Buffer
	globalManager = NewManager("glb", "1.0", "", &stdout, &stderr, os.Stdin, nil)
	var exiter recordingExiter
	globalManager.e = &exiter
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
