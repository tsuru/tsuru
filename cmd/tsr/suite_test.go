// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"sync/atomic"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	tTesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) SetUpSuite(c *gocheck.C) {
	config.ReadConfigFile("testdata/tsuru.conf")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

type CommandableProvisioner struct {
	tTesting.FakeProvisioner
	cmd *FakeCommand
}

func (p *CommandableProvisioner) Commands() []cmd.Command {
	if p.cmd == nil {
		p.cmd = &FakeCommand{}
	}
	return []cmd.Command{p.cmd}
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

func (c *FakeCommand) Run(*cmd.Context, *cmd.Client) error {
	atomic.AddInt32(&c.calls, 1)
	return nil
}
