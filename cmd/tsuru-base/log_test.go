// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"launchpad.net/gocheck"
	"net/http"
	"time"
)

func (s *S) TestFormatterUsesCurrentTimeZone(c *gocheck.C) {
	t := time.Now()
	logs := []log{
		{Date: t, Message: "Something happened", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	old := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = old
	}()
	formatter := logFormatter{}
	err = formatter.Format(&writer, data)
	c.Assert(err, gocheck.IsNil)
	tfmt := "2006-01-02 15:04:05 -0700"
	t = t.In(time.UTC)
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " Something happened\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru]:", "blue", "", "") + " Something happened again\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLog(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "app", Unit: "abcdef"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [app][abcdef]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	transport := testing.Transport{
		Message: string(result),
		Status:  http.StatusOK,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command.Flags().Parse(true, []string{"--app", "appName"})
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogWithUnparsableData(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"

	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	transport := testing.Transport{
		Message: string(result) + "\nunparseable data",
		Status:  http.StatusOK,
	}
	client := cmd.NewClient(&http.Client{Transport: &transport}, nil, manager)
	command.Flags().Parse(true, []string{"--app", "appName"})
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected += "Error: unparseable data"
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogWithoutTheFlag(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "app"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [app]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/apps/hitthelights/log" && req.Method == "GET" &&
				req.URL.Query().Get("lines") == "10"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogShouldReturnNilIfHasNoContent(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &testing.Transport{Message: "", Status: http.StatusNoContent}}, nil, manager)
	command.Flags().Parse(true, []string{"--app", "appName"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
}

func (s *S) TestAppLogInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines/-l numberOfLines] [--source/-s source] [--unit/-u unit] [--follow/-f]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
	c.Assert((&AppLog{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppLogBySource(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "tsuru"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--source", "mysource"})
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("source") == "mysource"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogByUnit(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru", Unit: "api"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "tsuru", Unit: "api"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru][api]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru][api]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--unit", "api"})
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("unit") == "api"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogWithLines(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "tsuru"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--lines", "12"})
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("lines") == "12"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogWithFollow(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	t := time.Now()
	logs := []log{
		{Date: t, Message: "creating app lost", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "app lost successfully created", Source: "tsuru"},
	}
	result, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	t = t.In(time.Local)
	tfmt := "2006-01-02 15:04:05 -0700"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--lines", "12", "-f"})
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(result), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Query().Get("lines") == "12" && req.URL.Query().Get("follow") == "1"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err = command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, expected)
}

func (s *S) TestAppLogFlagSet(c *gocheck.C) {
	command := AppLog{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--source", "tsuru", "--unit", "abcdef", "--lines", "12", "--app", "ashamed", "--follow"})
	source := flagset.Lookup("source")
	c.Check(source, gocheck.NotNil)
	c.Check(source.Name, gocheck.Equals, "source")
	c.Check(source.Usage, gocheck.Equals, "The log from the given source")
	c.Check(source.Value.String(), gocheck.Equals, "tsuru")
	c.Check(source.DefValue, gocheck.Equals, "")
	ssource := flagset.Lookup("s")
	c.Check(ssource, gocheck.NotNil)
	c.Check(ssource.Name, gocheck.Equals, "s")
	c.Check(ssource.Usage, gocheck.Equals, "The log from the given source")
	c.Check(ssource.Value.String(), gocheck.Equals, "tsuru")
	c.Check(ssource.DefValue, gocheck.Equals, "")
	unit := flagset.Lookup("unit")
	c.Check(unit, gocheck.NotNil)
	c.Check(unit.Name, gocheck.Equals, "unit")
	c.Check(unit.Usage, gocheck.Equals, "The log from the given unit")
	c.Check(unit.Value.String(), gocheck.Equals, "abcdef")
	c.Check(unit.DefValue, gocheck.Equals, "")
	sunit := flagset.Lookup("u")
	c.Check(sunit, gocheck.NotNil)
	c.Check(sunit.Name, gocheck.Equals, "u")
	c.Check(sunit.Usage, gocheck.Equals, "The log from the given unit")
	c.Check(sunit.Value.String(), gocheck.Equals, "abcdef")
	c.Check(sunit.DefValue, gocheck.Equals, "")
	lines := flagset.Lookup("lines")
	c.Check(lines, gocheck.NotNil)
	c.Check(lines.Name, gocheck.Equals, "lines")
	c.Check(lines.Usage, gocheck.Equals, "The number of log lines to display")
	c.Check(lines.Value.String(), gocheck.Equals, "12")
	c.Check(lines.DefValue, gocheck.Equals, "10")
	slines := flagset.Lookup("l")
	c.Check(slines, gocheck.NotNil)
	c.Check(slines.Name, gocheck.Equals, "l")
	c.Check(slines.Usage, gocheck.Equals, "The number of log lines to display")
	c.Check(slines.Value.String(), gocheck.Equals, "12")
	c.Check(slines.DefValue, gocheck.Equals, "10")
	app := flagset.Lookup("app")
	c.Check(app, gocheck.NotNil)
	c.Check(app.Name, gocheck.Equals, "app")
	c.Check(app.Usage, gocheck.Equals, "The name of the app.")
	c.Check(app.Value.String(), gocheck.Equals, "ashamed")
	c.Check(app.DefValue, gocheck.Equals, "")
	sapp := flagset.Lookup("a")
	c.Check(sapp, gocheck.NotNil)
	c.Check(sapp.Name, gocheck.Equals, "a")
	c.Check(sapp.Usage, gocheck.Equals, "The name of the app.")
	c.Check(sapp.Value.String(), gocheck.Equals, "ashamed")
	c.Check(sapp.DefValue, gocheck.Equals, "")
	follow := flagset.Lookup("follow")
	c.Check(follow, gocheck.NotNil)
	c.Check(follow.Name, gocheck.Equals, "follow")
	c.Check(follow.Usage, gocheck.Equals, "Follow logs")
	c.Check(follow.Value.String(), gocheck.Equals, "true")
	c.Check(follow.DefValue, gocheck.Equals, "false")
	sfollow := flagset.Lookup("f")
	c.Check(sfollow, gocheck.NotNil)
	c.Check(sfollow.Name, gocheck.Equals, "f")
	c.Check(sfollow.Usage, gocheck.Equals, "Follow logs")
	c.Check(sfollow.Value.String(), gocheck.Equals, "true")
	c.Check(sfollow.DefValue, gocheck.Equals, "false")
}
