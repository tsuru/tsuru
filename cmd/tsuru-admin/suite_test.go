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

var manager *cmd.Manager

func (s *S) SetUpSuite(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = cmd.NewManager("glb", version, header, &stdout, &stderr, os.Stdin, nil)
	s.recover = tTesting.SetTargetFile(c)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	tTesting.RollbackTargetFile(s.recover)
}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }

type AdminCommandableProvisioner struct {
	tTesting.FakeProvisioner
}

func (p *AdminCommandableProvisioner) AdminCommands() []cmd.Command {
	return []cmd.Command{&FakeAdminCommand{}}
}

type FakeAdminCommand struct{}

func (c *FakeAdminCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "fake-admin",
		Usage: "fake usage",
		Desc:  "fake desc",
	}
}

func (c *FakeAdminCommand) Run(*cmd.Context, *cmd.Client) error {
	return nil
}
