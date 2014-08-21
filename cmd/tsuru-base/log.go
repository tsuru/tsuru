// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"launchpad.net/gnuflag"
)

type AppLog struct {
	GuessingCommand
	fs     *gnuflag.FlagSet
	source string
	unit   string
	lines  int
	follow bool
}

func (c *AppLog) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines/-l numberOfLines] [--source/-s source] [--unit/-u unit] [--follow/-f]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
}

type logFormatter struct{}

func (logFormatter) Format(out io.Writer, data []byte) error {
	var logs []log
	err := json.Unmarshal(data, &logs)
	if err != nil {
		return err
	}
	for _, l := range logs {
		date := l.Date.In(time.Local).Format("2006-01-02 15:04:05 -0700")
		var prefix string
		if l.Unit != "" {
			prefix = fmt.Sprintf("%s [%s][%s]:", date, l.Source, l.Unit)
		} else {
			prefix = fmt.Sprintf("%s [%s]:", date, l.Source)
		}
		fmt.Fprintf(out, "%s %s\n", cmd.Colorfy(prefix, "blue", "", ""), l.Message)
	}
	return nil
}

type log struct {
	Date    time.Time
	Message string
	Source  string
	Unit    string
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
	if c.unit != "" {
		url = fmt.Sprintf("%s&unit=%s", url, c.unit)
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
	w := tsuruIo.NewStreamWriter(context.Stdout, logFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	unparsed := w.Remaining()
	if len(unparsed) > 0 {
		fmt.Fprintf(context.Stdout, "Error: %s", string(unparsed))
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
		c.fs.StringVar(&c.unit, "unit", "", "The log from the given unit")
		c.fs.StringVar(&c.unit, "u", "", "The log from the given unit")
		c.fs.BoolVar(&c.follow, "follow", false, "Follow logs")
		c.fs.BoolVar(&c.follow, "f", false, "Follow logs")
	}
	return c.fs
}
