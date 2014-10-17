// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	ttesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestMoveContainersInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "containers-move",
		Usage:   "containers-move <from host> <to host>",
		Desc:    "Move all containers from one host to another.\nThis command is especially useful for host maintenance.",
		MinArgs: 2,
	}
	c.Assert((&moveContainersCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestMoveContainersRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"from", "to"},
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"from": "from",
				"to":   "to",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/move" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestMoveContainerInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "container-move",
		Usage:   "container-move <container id> <to host>",
		Desc:    "Move specified container to another host.",
		MinArgs: 2,
	}
	c.Assert((&moveContainerCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestMoveContainerRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"contId", "toHost"},
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"to": "toHost",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/container/contId/move" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := moveContainerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestRebalanceContainersInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:    "containers-rebalance",
		Usage:   "containers-rebalance [--dry] [-y/--assume-yes]",
		Desc:    "Move containers creating a more even distribution between docker nodes.",
		MinArgs: 0,
	}
	c.Assert((&rebalanceContainersCmd{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestRebalanceContainersRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	expectedDry := "true"
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"dry": expectedDry,
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/rebalance" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Flags().Parse(true, []string{"--dry", "-y"})
	c.Assert(err, gocheck.IsNil)
	err = cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := "progress msg\n"
	c.Assert(stdout.String(), gocheck.Equals, expected)
	expectedDry = "false"
	cmd2 := rebalanceContainersCmd{}
	cmd2.Flags().Parse(true, []string{"-y"})
	err = cmd2.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRebalanceContainersRunAskingForConfirmation(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  bytes.NewBufferString("y"),
	}
	msg, _ := json.Marshal(progressLog{Message: "progress msg"})
	result := string(msg)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: result, Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			defer req.Body.Close()
			body, err := ioutil.ReadAll(req.Body)
			c.Assert(err, gocheck.IsNil)
			expected := map[string]string{
				"dry": "false",
			}
			result := map[string]string{}
			err = json.Unmarshal(body, &result)
			c.Assert(expected, gocheck.DeepEquals, result)
			return req.URL.Path == "/docker/containers/rebalance" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you want to rebalance containers? (y/n) progress msg\n")
	cmd2 := rebalanceContainersCmd{}
	err = cmd2.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRebalanceContainersRunGivingUp(c *gocheck.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stdin:  bytes.NewBufferString("n\n"),
	}
	cmd := rebalanceContainersCmd{}
	err := cmd.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you want to rebalance containers? (y/n) Abort.\n")
}

func (s *S) TestFixContainersCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Stdout: &buf, Stderr: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/fix-containers" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &buf, &buf, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	cmd := fixContainersCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestFixContainersCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "fix-containers",
		Usage: "fix-containers",
		Desc:  "Fix containers that are broken in the cluster.",
	}
	command := fixContainersCmd{}
	info := command.Info()
	c.Assert(*info, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHToContainerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "ssh",
		Usage:   "ssh <[-a/--app <appname>]|[container-id]>",
		Desc:    "Open an SSH shell to the given container, or to one of the containers of the given app.",
		MinArgs: 0,
	}
	var command sshToContainerCmd
	info := command.Info()
	c.Assert(*info, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHToContainerCmdRun(c *gocheck.C) {
	var closeClientConn func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docker/ssh/af3332d" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
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
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	tokenRecover := ttesting.SetTokenFile(c, []byte("abc123"))
	defer ttesting.RollbackFile(tokenRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello my friend\nglad to see you here\n")
}

func (s *S) TestSSHToContainerCmdRunWithApp(c *gocheck.C) {
	guesser := testing.FakeGuesser{Name: "myapp"}
	var closeClientConn func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps/myapp" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
			app := apiApp{Units: []unit{{Name: "abc123f0"}, {Name: "abc123f1"}}}
			json.NewEncoder(w).Encode(app)
		} else if r.URL.Path == "/docker/ssh/abc123f0" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
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
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	tokenRecover := ttesting.SetTokenFile(c, []byte("abc123"))
	defer ttesting.RollbackFile(tokenRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	command.GuessingCommand = cmd.GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, gocheck.IsNil)
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(http.DefaultClient, &context, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello my friend\nglad to see you here\n")
}

func (s *S) TestSSHToContainerAppNotFound(c *gocheck.C) {
	guesser := testing.FakeGuesser{Name: "myapp"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps/myapp" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
			http.Error(w, "app not found", http.StatusNotFound)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	tokenRecover := ttesting.SetTokenFile(c, []byte("abc123"))
	defer ttesting.RollbackFile(tokenRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	command.GuessingCommand = cmd.GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, gocheck.IsNil)
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(http.DefaultClient, &context, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "app not found\n")
}

func (s *S) TestSSHToContainerAppWithNoUnits(c *gocheck.C) {
	guesser := testing.FakeGuesser{Name: "myapp"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps/myapp" && r.Method == "GET" && r.Header.Get("Authorization") == "bearer abc123" {
			w.Write([]byte(`{"Units":[]}`))
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	tokenRecover := ttesting.SetTokenFile(c, []byte("abc123"))
	defer ttesting.RollbackFile(tokenRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	command.GuessingCommand = cmd.GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{"-a", "myapp"})
	c.Assert(err, gocheck.IsNil)
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(http.DefaultClient, &context, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "app must have at least one container")
}

func (s *S) TestSSHToContainerNoAppNoArg(c *gocheck.C) {
	guesser := testing.FailingFakeGuesser{}
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	command.GuessingCommand = cmd.GuessingCommand{G: &guesser}
	err := command.Flags().Parse(true, []string{})
	c.Assert(err, gocheck.IsNil)
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(http.DefaultClient, &context, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "you need to specify either the container id or the app name")
}

func (s *S) TestSSHToContainerCmdNoToken(c *gocheck.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "You must provide a valid Authorization header", http.StatusUnauthorized)
	}))
	defer server.Close()
	target := "http://" + server.Listener.Addr().String()
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	var buf bytes.Buffer
	context := cmd.Context{
		Args:   []string{"af3332d"},
		Stdout: &buf,
		Stderr: &buf,
		Stdin:  &buf,
	}
	var command sshToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err.Error(), gocheck.Equals, "HTTP/1.1 401 Unauthorized")
}

func (s *S) TestSSHToContainerCmdSmallData(c *gocheck.C) {
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
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello")
}

func (s *S) TestSSHToContainerCmdLongNoNewLine(c *gocheck.C) {
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
	targetRecover := ttesting.SetTargetFile(c, []byte(target))
	defer ttesting.RollbackFile(targetRecover)
	var stdout, stderr, stdin bytes.Buffer
	context := cmd.Context{
		Args:   []string{"af3332d"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
	}
	var command sshToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestSSHToContainerCmdConnectionRefused(c *gocheck.C) {
	server := httptest.NewServer(nil)
	addr := server.Listener.Addr().String()
	server.Close()
	targetRecover := ttesting.SetTargetFile(c, []byte("http://"+addr))
	defer ttesting.RollbackFile(targetRecover)
	tokenRecover := ttesting.SetTokenFile(c, []byte("abc123"))
	defer ttesting.RollbackFile(tokenRecover)
	var buf bytes.Buffer
	context := cmd.Context{
		Args:   []string{"af3332d"},
		Stdout: &buf,
		Stderr: &buf,
		Stdin:  &buf,
	}
	var command sshToContainerCmd
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.NotNil)
	opErr, ok := err.(*net.OpError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(opErr.Net, gocheck.Equals, "tcp")
	c.Assert(opErr.Op, gocheck.Equals, "dial")
	c.Assert(opErr.Addr.String(), gocheck.Equals, addr)
}
