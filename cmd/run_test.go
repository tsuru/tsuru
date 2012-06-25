package cmd

import (
	"bytes"
	. "launchpad.net/gocheck"
	"net/http"
)

func (s *S) TestAppRun(c *C) {
	expected := "http.go		http_test.go"
	context := Context{[]string{}, []string{"ble", "ls"}, manager.Stdout, manager.Stderr}
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
	client := NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestAppRunShouldUseAllSubsequentArgumentsAsArgumentsToTheGivenCommand(c *C) {
	expected := "-rw-r--r--  1 f  staff  119 Apr 26 18:23 http.go"
	context := Context{[]string{}, []string{"ble", "ls", "-l"}, manager.Stdout, manager.Stderr}
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
	client := NewClient(&http.Client{Transport: trans})
	err := (&AppRun{}).Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(manager.Stdout.(*bytes.Buffer).String(), Equals, expected)
}

func (s *S) TestInfoAppRun(c *C) {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	expected := &Info{
		Name:    "run",
		Usage:   `run appname command commandarg1 commandarg2 ... commandargn`,
		Desc:    desc,
		MinArgs: 1,
	}
	command := AppRun{}
	c.Assert(command.Info(), DeepEquals, expected)
}
