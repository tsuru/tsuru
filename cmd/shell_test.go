// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/cmd/cmdtest"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestShellToContainerCmdInfo(c *gocheck.C) {
	expected := Info{
		Name:    "app-shell",
		Usage:   "app-shell [container-id] -a/--app <appname>",
		Desc:    "Open a remote shell to the given container, or to one of the containers of the given app.",
		MinArgs: 0,
	}
	var command ShellToContainerCmd
	info := command.Info()
	c.Assert(*info, gocheck.DeepEquals, expected)
}

func (s *S) TestShellToContainerCmdRunWithApp(c *gocheck.C) {
	var closeClientConn func()
	guesser := cmdtest.FakeGuesser{Name: "myapp"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps/myapp/shell" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
			conn, _, err := w.(http.Hijacker).Hijack()
			c.Assert(err, gocheck.IsNil)
			conn.Write([]byte("hello my friend\n"))
			conn.Write([]byte("glad to see you here\n"))
			closeClientConn()
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()
	closeClientConn = server.CloseClientConnections
	target := "http://" + server.Listener.Addr().String()
	targetRecover := testing.SetTargetFile(c, []byte(target))
	defer testing.RollbackFile(targetRecover)
	tokenRecover := testing.SetTokenFile(c, []byte("abc123"))
	defer testing.RollbackFile(tokenRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	command.GuessingCommand = GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, gocheck.IsNil)
	manager := NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := NewClient(http.DefaultClient, &context, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello my friend\nglad to see you here\n")
}

func (s *S) TestShellToContainerCmdNoToken(c *gocheck.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "You must provide a valid Authorization header", http.StatusUnauthorized)
	}))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	targetRecover := testing.SetTargetFile(c, []byte(target))
	defer testing.RollbackFile(targetRecover)
	var stdin, stdout, stderr bytes.Buffer
	context := Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "HTTP/1.1 401")
}

func (s *S) TestShellToContainerCmdSmallData(c *gocheck.C) {
	var closeClientConn func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		c.Assert(err, gocheck.IsNil)
		conn.Write([]byte("hello"))
		closeClientConn()
	}))
	defer server.Close()
	closeClientConn = server.CloseClientConnections
	target := "http://" + server.Listener.Addr().String()
	targetRecover := testing.SetTargetFile(c, []byte(target))
	defer testing.RollbackFile(targetRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello")
}

func (s *S) TestShellToContainerCmdLongNoNewLine(c *gocheck.C) {
	var closeClientConn func()
	expected := fmt.Sprintf("%0200s", "x")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		c.Assert(err, gocheck.IsNil)
		conn.Write([]byte(expected))
		closeClientConn()
	}))
	defer server.Close()
	closeClientConn = server.CloseClientConnections
	target := "http://" + server.Listener.Addr().String()
	targetRecover := testing.SetTargetFile(c, []byte(target))
	defer testing.RollbackFile(targetRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command ShellToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestShellToContainerCmdConnectionRefused(c *gocheck.C) {
	server := httptest.NewServer(nil)
	addr := server.Listener.Addr().String()
	server.Close()
	targetRecover := testing.SetTargetFile(c, []byte("http://"+addr))
	defer testing.RollbackFile(targetRecover)
	tokenRecover := testing.SetTokenFile(c, []byte("abc123"))
	defer testing.RollbackFile(tokenRecover)
	var buf bytes.Buffer
	context := Context{
		Args:   []string{"af3332d"},
		Stdout: &buf,
		Stderr: &buf,
		Stdin:  &buf,
	}
	var command ShellToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	opErr, ok := err.(*net.OpError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(opErr.Net, gocheck.Equals, "tcp")
	c.Assert(opErr.Op, gocheck.Equals, "dial")
	c.Assert(opErr.Addr.String(), gocheck.Equals, addr)
}
