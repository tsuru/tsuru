// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"golang.org/x/net/websocket"
	"gopkg.in/check.v1"
)

func buildHandler(content []byte) websocket.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		conn.Write(content)
		conn.Close()
	})
}

func (s *S) TestShellToContainerCmdInfo(c *check.C) {
	var command ShellToContainerCmd
	info := command.Info()
	c.Assert(info, check.NotNil)
}

func (s *S) TestShellToContainerCmdRunWithApp(c *check.C) {
	guesser := cmdtest.FakeGuesser{Name: "myapp"}
	server := httptest.NewServer(buildHandler([]byte("hello my friend\nglad to see you here\n")))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	os.Setenv("TSURU_TARGET", target)
	defer os.Unsetenv("TSURU_TARGET")
	os.Setenv("TSURU_TOKEN", "abc123")
	defer os.Unsetenv("TSURU_TOKEN")
	var stdout, stderr, stdin bytes.Buffer
	context := Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	command.GuessingCommand = GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, check.IsNil)
	mngr := NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := NewClient(http.DefaultClient, &context, mngr)
	err = command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello my friend\nglad to see you here\n")
}

func (s *S) TestShellToContainerWithUnit(c *check.C) {
	guesser := cmdtest.FakeGuesser{Name: "myapp"}
	server := httptest.NewServer(buildHandler([]byte("hello my friend\nglad to see you here\n")))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	os.Setenv("TSURU_TARGET", target)
	defer os.Unsetenv("TSURU_TARGET")
	os.Setenv("TSURU_TOKEN", "abc123")
	defer os.Unsetenv("TSURU_TOKEN")
	var stdout, stderr, stdin bytes.Buffer
	context := Context{
		Args:   []string{"containerid"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	command.GuessingCommand = GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, check.IsNil)
	mngr := NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := NewClient(http.DefaultClient, &context, mngr)
	err = command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello my friend\nglad to see you here\n")
}

func (s *S) TestShellToContainerCmdConnectionRefused(c *check.C) {
	server := httptest.NewServer(nil)
	addr := server.Listener.Addr().String()
	server.Close()
	os.Setenv("TSURU_TARGET", "http://"+addr)
	defer os.Unsetenv("TSURU_TARGET")
	os.Setenv("TSURU_TOKEN", "abc123")
	defer os.Unsetenv("TSURU_TOKEN")
	var buf bytes.Buffer
	context := Context{
		Args:   []string{"af3332d"},
		Stdout: &buf,
		Stderr: &buf,
		Stdin:  &buf,
	}
	var command ShellToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, check.NotNil)
}
