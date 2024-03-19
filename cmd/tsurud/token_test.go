// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
)

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
	command := &tsurudCommand{Command: createRootUserCmd{}}
	command.Flags().Parse(true, []string{"--config", "testdata/tsuru.conf"})
	err := command.Run(&context)
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
