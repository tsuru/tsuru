// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"net/http"
)

type logRemove struct {
	tsuru.GuessingCommand
}

func (c *logRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "log-remove",
		Usage:   "log-remove [--app appname]",
		Desc:    `remove all app logs.`,
		MinArgs: 0,
	}
}

func (c *logRemove) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	uri := "/logs"
	if err == nil {
		uri += "?app=" + appName
	}
	url, err := cmd.GetURL(uri)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Logs successfully removed!\n")
	return nil
}
