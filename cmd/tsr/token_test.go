// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"net/http"
	"os"

	"github.com/tsuru/tsuru/cmd"
	"gopkg.in/check.v1"
)

func (s *S) TestTokenCmdInfo(c *check.C) {
	expected := &cmd.Info{
		Name:    "token",
		Usage:   "token",
		Desc:    "Generates a tsuru token.",
		MinArgs: 0,
	}
	c.Assert(tokenCmd{}.Info(), check.DeepEquals, expected)
}

func (s *S) TestTokenCmdIsACommand(c *check.C) {
	var _ cmd.Command = &tokenCmd{}
}

func (s *S) TestTokenRun(c *check.C) {
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
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Not(check.Equals), "")
}
