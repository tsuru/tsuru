// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
	"net/http"
	"strings"
	"time"
)

func (s *S) TestJsonWriter(c *gocheck.C) {
	t := time.Now()
	logs := []log{
		{Date: t, Message: "Something happened", Source: "tsuru"},
		{Date: t.Add(2 * time.Hour), Message: "Something happened again", Source: "tsuru"},
	}
	b, err := json.Marshal(logs)
	c.Assert(err, gocheck.IsNil)
	var buf bytes.Buffer
	w := jsonWriter{&buf}
	n, err := w.Write(b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(b))
	tfmt := "2006-01-02 15:04:05"
	expected := cmd.Colorfy(t.Format(tfmt)+" [tsuru]:", "blue", "", "") + " Something happened\n"
	expected = expected + cmd.Colorfy(t.Add(2*time.Hour).Format(tfmt)+" [tsuru]:", "blue", "", "") + " Something happened again\n"
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestJsonWriterInvalidJson(c *gocheck.C) {
	var buf bytes.Buffer
	w := jsonWriter{&buf}
	b := []byte("-----")
	n, err := w.Write(b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(b))
	c.Assert(buf.String(), gocheck.Equals, "")
}

func (s *S) TestAppLog(c *gocheck.C) {
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
	command.Flags().Parse(true, []string{"--app", "appName"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestAppLogWithoutTheFlag(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, nil)
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Path == "/apps/hitthelights/log" && req.Method == "GET" &&
				req.URL.Query().Get("lines") == "10"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestAppLogShouldReturnNilIfHasNoContent(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	command := AppLog{}
	client := cmd.NewClient(&http.Client{Transport: &transport{msg: "", status: http.StatusNoContent}}, nil, manager)
	command.Flags().Parse(true, []string{"--app", "appName"})
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
}

func (s *S) TestAppLogInfo(c *gocheck.C) {
	expected := &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines/-l numberOfLines] [--source/-s source] [--follow/-f]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
	c.Assert((&AppLog{}).Info(), gocheck.DeepEquals, expected)
}

func (s *S) TestAppLogBySource(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--source", "mysource"})
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
	c.Assert(err, gocheck.IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestAppLogWithLines(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--lines", "12"})
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Query().Get("lines") == "12"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestAppLogWithFollow(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	result := `[{"Source":"tsuru","Date":"2012-06-20T11:17:22.75-03:00","Message":"creating app lost"},{"Source":"tsuru","Date":"2012-06-20T11:17:22.753-03:00","Message":"app lost successfully created"}]`
	expected := cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " creating app lost\n"
	expected = expected + cmd.Colorfy("2012-06-20 11:17:22 [tsuru]:", "blue", "", "") + " app lost successfully created\n"
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	fake := &FakeGuesser{name: "hitthelights"}
	command := AppLog{GuessingCommand: GuessingCommand{G: fake}}
	command.Flags().Parse(true, []string{"--lines", "12", "-f"})
	trans := &conditionalTransport{
		transport{
			msg:    result,
			status: http.StatusOK,
		},
		func(req *http.Request) bool {
			return req.URL.Query().Get("lines") == "12" && req.URL.Query().Get("follow") == "1"
		},
	}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	err := command.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
	got := stdout.String()
	got = strings.Replace(got, "-0300 -0300", "-0300 BRT", -1)
	c.Assert(got, gocheck.Equals, expected)
}

func (s *S) TestAppLogFlagSet(c *gocheck.C) {
	command := AppLog{}
	flagset := command.Flags()
	flagset.Parse(true, []string{"--source", "tsuru", "--lines", "12", "--app", "ashamed", "--follow"})
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
