// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"strings"

	"launchpad.net/gocheck"
)

func (s *S) TestConfirmationConfirmFalse(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := "Are you sure you wanna do it? (y/n) Abort.\n"
	context := Context{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("n\n")}
	cmd := ConfirmationCommand{}
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, gocheck.Equals, false)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestConfirmationConfirmTrue(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := "Are you sure you wanna do it? (y/n) "
	context := Context{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("y\n")}
	cmd := ConfirmationCommand{}
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestConfirmationConfirmWithFlagShort(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := Context{Stdout: &stdout, Stderr: &stderr}
	cmd := ConfirmationCommand{}
	cmd.Flags().Parse(true, []string{"-y"})
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "")
}

func (s *S) TestConfirmationConfirmWithFlagLong(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := Context{Stdout: &stdout, Stderr: &stderr}
	cmd := ConfirmationCommand{}
	cmd.Flags().Parse(true, []string{"--assume-yes"})
	result := cmd.Confirm(&context, "Are you sure you wanna do it?")
	c.Assert(result, gocheck.Equals, true)
	c.Assert(stdout.String(), gocheck.Equals, "")
}
