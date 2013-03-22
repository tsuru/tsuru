// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
	"time"
)

type AppLog struct {
	GuessingCommand
	fs *gnuflag.FlagSet
}

func (c *AppLog) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "log",
		Usage: "log [--app appname] [--lines numberOfLines] [--source source]",
		Desc: `show logs for an app.

If you don't provide the app name, tsuru will try to guess it. The default number of lines is 10.`,
		MinArgs: 0,
	}
}

type log struct {
	Date    time.Time
	Message string
	Source  string
}

func (c *AppLog) Run(context *cmd.Context, client cmd.Doer) error {
	var err error
	appName := c.fs.Lookup("app").Value.String()
	if appName == "" {
		appName, err = c.Guess()
		if err != nil {
			return err
		}
	}
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s/log?lines=%s", appName, c.fs.Lookup("lines").Value.String()))
	if err != nil {
		return err
	}
	if source := c.fs.Lookup("source").Value.String(); source != "" {
		url = fmt.Sprintf("%s&source=%s", url, source)
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
	result, err := ioutil.ReadAll(response.Body)
	logs := []log{}
	err = json.Unmarshal(result, &logs)
	if err != nil {
		return err
	}
	for _, l := range logs {
		date := l.Date.Format("2006-01-02 15:04:05")
		prefix := fmt.Sprintf("%s [%s]:", date, l.Source)
		msg := fmt.Sprintf("%s %s\n", cmd.Colorfy(prefix, "blue", "", ""), l.Message)
		context.Stdout.Write([]byte(msg))
	}
	return err
}

func (c *AppLog) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("log", gnuflag.ContinueOnError)
		c.fs.Int("lines", 10, "The number of log lines to display")
		c.fs.String("source", "", "The log from the given source")
		AddAppFlag(c.fs)
	}
	return c.fs
}
