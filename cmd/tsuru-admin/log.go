// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"net/http"
)

type LogRemove struct{}

func (c *LogRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "log-remove",
		Usage:   "log-remove",
		Desc:    `remove all app logs.`,
		MinArgs: 0,
	}
}

func (c *LogRemove) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/logs")
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
