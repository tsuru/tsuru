// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"net/http"
	"os"

	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gocheck"
)

func (s *S) TestTokenCmdInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "token",
		Usage:   "token",
		Desc:    "Generates a tsuru token.",
		MinArgs: 0,
	}
	c.Assert(tokenCmd{}.Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestTokenCmdIsACommand(c *gocheck.C) {
	var _ cmd.Command = &tokenCmd{}
}

func (s *S) TestTokenRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	manager := cmd.NewManager("glb", "", "", &stdout, &stderr, os.Stdin, nil)
	client := cmd.NewClient(&http.Client{}, nil, manager)
	command := tokenCmd{}
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Not(gocheck.Equals), "")
}
