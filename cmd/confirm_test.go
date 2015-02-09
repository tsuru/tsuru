// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"strings"

	"gopkg.in/check.v1"
)

func (s *S) TestConfirmationConfirmFalse(c *check.C) {
	var stdout, stderr bytes.Buffer
	expected := "Are you sure you wanna do it? (y/n) Abort.\n"
	context := Context{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("n\n")}
	cmd := ConfirmationCommand{}
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, check.Equals, false)
	c.Assert(stdout.String(), check.Equals, expected)
}

func (s *S) TestConfirmationConfirmTrue(c *check.C) {
	var stdout, stderr bytes.Buffer
	expected := "Are you sure you wanna do it? (y/n) "
	context := Context{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("y\n")}
	cmd := ConfirmationCommand{}
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, check.Equals, true)
	c.Assert(stdout.String(), check.Equals, expected)
}

func (s *S) TestConfirmationConfirmWithFlagShort(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := Context{Stdout: &stdout, Stderr: &stderr}
	cmd := ConfirmationCommand{}
	cmd.Flags().Parse(true, []string{"-y"})
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, check.Equals, true)
	c.Assert(stdout.String(), check.Equals, "")
}

func (s *S) TestConfirmationConfirmWithFlagLong(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := Context{Stdout: &stdout, Stderr: &stderr}
	cmd := ConfirmationCommand{}
	cmd.Flags().Parse(true, []string{"--assume-yes"})
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, check.Equals, true)
	c.Assert(stdout.String(), check.Equals, "")
}
