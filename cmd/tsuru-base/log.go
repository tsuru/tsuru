// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
	"time"
)

type AppLog struct {
	GuessingCommand
	fs     *gnuflag.FlagSet
	source string
	lines  int
	follow bool
}

func (c *AppLog) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines/-l numberOfLines] [--source/-s source] [--follow/-f]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
}

type jsonWriter struct {
	w io.Writer
	b []byte
}

func (w *jsonWriter) Write(b []byte) (int, error) {
	var logs []log
	w.b = append(w.b, b...)
	err := json.Unmarshal(w.b, &logs)
	if err != nil {
		return len(b), nil
	}
	for _, l := range logs {
		date := l.Date.In(time.Local).Format("2006-01-02 15:04:05 -0700")
		prefix := fmt.Sprintf("%s [%s]:", date, l.Source)
		fmt.Fprintf(w.w, "%s %s\n", cmd.Colorfy(prefix, "blue", "", ""), l.Message)
	}
	w.b = nil
	return len(b), nil
}

type log struct {
	Date    time.Time
	Message string
	Source  string
}

func (c *AppLog) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/log?lines=%d", appName, c.lines))
	if err != nil {
		return err
	}
	if c.source != "" {
		url = fmt.Sprintf("%s&source=%s", url, c.source)
	}
	if c.follow {
		url += "&follow=1"
	}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	defer response.Body.Close()
	w := jsonWriter{w: context.Stdout}
	for n, err := io.Copy(&w, response.Body); n > 0 && err == nil; n, err = io.Copy(&w, response.Body) {
	}
	return nil
}

func (c *AppLog) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.GuessingCommand.Flags()
		c.fs.IntVar(&c.lines, "lines", 10, "The number of log lines to display")
		c.fs.IntVar(&c.lines, "l", 10, "The number of log lines to display")
		c.fs.StringVar(&c.source, "source", "", "The log from the given source")
		c.fs.StringVar(&c.source, "s", "", "The log from the given source")
		c.fs.BoolVar(&c.follow, "follow", false, "Follow logs")
		c.fs.BoolVar(&c.follow, "f", false, "Follow logs")
	}
	return c.fs
}