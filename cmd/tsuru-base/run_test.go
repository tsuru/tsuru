// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestAppRun(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := "http.go		http_test.go"
	context := cmd.Context{
		Args:   []string{"ls"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := runMessage{Message: expected}
	result, err := json.Marshal(msg)
	c.Assert(err, gocheck.IsNil)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: string(result),
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			b := make([]byte, 2)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AppRun{}
	command.Flags().Parse(true, []string{"--app", "ble"})
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRunShouldUseAllSubsequentArgumentsAsArgumentsToTheGivenCommand(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go\n"
	context := cmd.Context{
		Args:   []string{"ls", "-l"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := runMessage{Message: expected}
	result, err := json.Marshal(msg)
	c.Assert(err, gocheck.IsNil)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: string(result) + "\n" + string(result),
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			b := make([]byte, 5)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls -l"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	command := AppRun{}
	command.Flags().Parse(true, []string{"--app", "ble"})
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected+expected)
}

func (s *S) TestAppRunWithoutTheFlag(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	expected := "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go"
	context := cmd.Context{
		Args:   []string{"ls", "-lh"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := runMessage{Message: expected}
	result, err := json.Marshal(msg)
	c.Assert(err, gocheck.IsNil)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: string(result),
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			b := make([]byte, 6)
			req.Body.Read(b)
			return req.URL.Path == "/apps/bla/run" && string(b) == "ls -lh"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "bla"}
	command := AppRun{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppRunShouldReturnErrorWhenCommandGoWrong(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Args:   []string{"cmd_error"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	msg := runMessage{Error: "command doesn't exist."}
	result, err := json.Marshal(msg)
	c.Assert(err, gocheck.IsNil)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{
			Message: string(result),
			Status:  http.StatusOK,
		},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/apps/bla/run"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	fake := &FakeGuesser{name: "bla"}
	command := AppRun{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.ErrorMatches, "command doesn't exist.")
}

func (s *S) TestAppRunInfo(c *gocheck.C) {
	desc := `run a command in all instances of the app, and prints the output.

If you use the '--once' flag tsuru will run the command only in one unit.

If you don't provide the app name, tsuru will try to guess it.
`
	expected := &cmd.Info{
		Name:    "run",
		Usage:   `run <command> [commandarg1] [commandarg2] ... [commandargn] [--app appname] [--once]`,
		Desc:    desc,
		MinArgs: 1,
	}
	command := AppRun{}
	c.Assert(command.Info(), gocheck.DeepEquals, expected)
}
