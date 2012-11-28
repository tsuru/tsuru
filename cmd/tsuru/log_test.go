// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	. "launchpad.net/gocheck"
	"net/http"
	"strings"
)

func (s *S) TestAppLog(c *C) {
	*appName = "appName"
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"app","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [app]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: result, status: http.StatusOK}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppLogWithoutTheFlag(c *C) {
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand{g: fake}}
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/hitthelights/log" && req.Method == "GET"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppLogShouldReturnNilIfHasNoContent(c *C) {
	*appName = "appName"
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	c.Assert(stdout.String(), Equals, "")
}

func (s *S) TestAppLogInfo(c *C) {
	expected := &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines numberOfLines] [--source source]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
	c.Assert((&AppLog{}).Info(), DeepEquals, expected)
}

func (s *S) TestAppLogBySource(c *C) {
	*logSource = "mysource"
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand{g: fake}}
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Query().Get("source") == "mysource"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, Equals, expected)
}
