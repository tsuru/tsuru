// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	tmpDir string
	env    *Environment
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	s.tmpDir, err = ioutil.TempDir("", "tsuru-integration")
	c.Assert(err, check.IsNil)
	err = os.Setenv("HOME", s.tmpDir)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	err := os.RemoveAll(s.tmpDir)
	c.Assert(err, check.IsNil)
}

func retry(timeout time.Duration, fn func() bool) bool {
	timeoutTimer := time.After(timeout)
	for {
		if fn() {
			return true
		}
		select {
		case <-time.After(5 * time.Second):
		case <-timeoutTimer:
			return false
		}
	}
}
