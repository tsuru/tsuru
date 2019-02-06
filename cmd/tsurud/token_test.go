// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"net/http"
	"os"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
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

func (s *S) TestCreateRootUserCmdInfo(c *check.C) {
	c.Assert((&createRootUserCmd{}).Info(), check.NotNil)
}

func (s *S) TestCreateRootUserCmdRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	reader := strings.NewReader("foo123\nfoo123\n")
	context := cmd.Context{
		Args:   []string{"my@user.com"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  reader,
	}
	manager := cmd.NewManager("glb", "", "", &stdout, &stderr, os.Stdin, nil)
	client := cmd.NewClient(&http.Client{}, nil, manager)
	command := createRootUserCmd{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Password: \nConfirm: \nRoot user successfully created.\n")
	u, err := auth.GetUserByEmail("my@user.com")
	c.Assert(err, check.IsNil)
	perms, err := u.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.HasLen, 2)
	c.Assert(perms[0].Scheme, check.Equals, permission.PermUser)
	c.Assert(perms[1].Scheme, check.Equals, permission.PermAll)
}
