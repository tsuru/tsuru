// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/testing"
	fs_test "github.com/globocom/tsuru/fs/testing"
	"launchpad.net/gocheck"
	"net/http"
	"os/user"
	"path"
)

func (s *S) TestKeyAdd(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	u, err := user.Current()
	c.Assert(err, gocheck.IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
	expected := fmt.Sprintf("Key %q successfully added!\n", p)
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "success", Status: http.StatusOK}}, nil, manager)
	fs := fs_test.RecordingFs{FileContent: "user-key"}
	command := KeyAdd{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
	c.Assert(fs.HasAction("open "+p), gocheck.Equals, true)
}

func (s *S) TestKeyAddSpecifyingKeyFile(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	u, err := user.Current()
	c.Assert(err, gocheck.IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_dsa.pub")
	expected := fmt.Sprintf("Key %q successfully added!\n", p)
	context := cmd.Context{
		Args:   []string{p},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "success", Status: http.StatusOK}}, nil, manager)
	fs := fs_test.RecordingFs{FileContent: "user-key"}
	command := KeyAdd{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
	c.Assert(fs.HasAction("open "+p), gocheck.Equals, true)
}

func (s *S) TestKeyAddReturnErrorIfTheKeyDoesNotExist(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FileNotFoundFs{RecordingFs: fs_test.RecordingFs{}}
	command := KeyAdd{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "You need to have a public rsa key")
}

func (s *S) TestKeyAddReturnsProperErrorIfTheGivenKeyFileDoesNotExist(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"/unknown/key.pub"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FileNotFoundFs{RecordingFs: fs_test.RecordingFs{}}
	command := KeyAdd{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "File /unknown/key.pub does not exist!")
	c.Assert(context.Stderr.(*bytes.Buffer).String(), gocheck.Equals, "File /unknown/key.pub does not exist!\n")
}

func (s *S) TestKeyAddError(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"/unknown/key.pub"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FailureFs{
		RecordingFs: fs_test.RecordingFs{},
		Err:         errors.New("what happened?"),
	}
	command := KeyAdd{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "what happened?")
}

func (s *S) TestInfoKeyAdd(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "key-add",
		Usage:   "key-add [path/to/key/file.pub]",
		Desc:    "add your public key ($HOME/.ssh/id_rsa.pub by default).",
		MinArgs: 0,
	}
	c.Assert((&KeyAdd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestKeyRemove(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	u, err := user.Current()
	c.Assert(err, gocheck.IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_rsa.pub")
	expected := fmt.Sprintf("Key %q successfully removed!\n", p)
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "success", Status: http.StatusOK}}, nil, manager)
	fs := fs_test.RecordingFs{FileContent: "user-key"}
	command := KeyRemove{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
	c.Assert(fs.HasAction("open "+p), gocheck.Equals, true)
}

func (s *S) TestKeyRemoveSpecifyingKeyFile(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	u, err := user.Current()
	c.Assert(err, gocheck.IsNil)
	p := path.Join(u.HomeDir, ".ssh", "id_dsa.pub")
	expected := fmt.Sprintf("Key %q successfully removed!\n", p)
	context := cmd.Context{
		Args:   []string{p},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "success", Status: http.StatusOK}}, nil, manager)
	fs := fs_test.RecordingFs{FileContent: "user-key"}
	command := KeyRemove{keyReader{fsystem: &fs}}
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
	c.Assert(fs.HasAction("open "+p), gocheck.Equals, true)
}

func (s *S) TestKeyRemoveReturnErrorIfTheKeyDoesNotExist(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FileNotFoundFs{RecordingFs: fs_test.RecordingFs{}}
	command := KeyRemove{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "You need to have a public rsa key")
}

func (s *S) TestKeyRemoveReturnProperErrorIfTheGivenKeyFileDoesNotExist(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"/unknown/key.pub"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FileNotFoundFs{RecordingFs: fs_test.RecordingFs{}}
	command := KeyRemove{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "File /unknown/key.pub does not exist!")
	c.Assert(context.Stderr.(*bytes.Buffer).String(), gocheck.Equals, err.Error()+"\n")
}

func (s *S) TestKeyRemoveError(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"/unknown/key.pub"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fs := fs_test.FailureFs{
		RecordingFs: fs_test.RecordingFs{},
		Err:         errors.New("what happened?"),
	}
	command := KeyRemove{keyReader{fsystem: &fs}}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "what happened?")
}

func (s *S) TestInfoKeyRemove(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "key-remove",
		Usage:   "key-remove [path/to/key/file.pub]",
		Desc:    "remove your public key ($HOME/.id_rsa.pub by default).",
		MinArgs: 0,
	}
	c.Assert((&KeyRemove{}).Info(), gocheck.DeepEquals, expected)
}
