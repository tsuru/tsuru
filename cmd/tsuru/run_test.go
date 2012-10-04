package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAppRun(c *C) {
	var stdout, stderr bytes.Buffer
	expected := "http.go		http_test.go"
	context := cmd.Context{
		Args:   []string{"ble", "ls"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg: "http.go		http_test.go",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			b := make([]byte, 2)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestAppRunShouldUseAllSubsequentArgumentsAsArgumentsToTheGivenCommand(c *C) {
	var stdout, stderr bytes.Buffer
	expected := "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go"
	context := cmd.Context{
		Args:   []string{"ble", "ls", "-l"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	trans := &conditionalTransport{
		transport{
			msg:    "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go",
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			b := make([]byte, 5)
			req.Body.Read(b)
			return req.URL.Path == "/apps/ble/run" && string(b) == "ls -l"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, expected)
}

func (s *S) TestInfoAppRun(c *C) {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	expected := &cmd.Info{
		Name:    "run",
		Usage:   `run <appname> <command> [commandarg1] [commandarg2] ... [commandargn]`,
		Desc:    desc,
		MinArgs: 1,
	}
	command := AppRun{}
	c.Assert(command.Info(), DeepEquals, expected)
}
