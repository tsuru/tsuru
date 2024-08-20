// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"sync/atomic"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db/storagev2"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpTest(c *check.C) {
	config.ReadConfigFile("testdata/tsuru.conf")
	storagev2.ClearAllCollections(nil)
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

type FakeCommand struct {
	calls int32
}

func (c *FakeCommand) Calls() int32 {
	return atomic.LoadInt32(&c.calls)
}

func (c *FakeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "fake",
		Usage: "fake fake",
		Desc:  "do nothing",
	}
}

func (c *FakeCommand) Run(*cmd.Context) error {
	atomic.AddInt32(&c.calls, 1)
	return nil
}
