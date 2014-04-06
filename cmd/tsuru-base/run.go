// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
	"strings"
)

type AppRun struct {
	GuessingCommand
	once bool
}

func (c *AppRun) Info() *cmd.Info {
	desc := `run a command in all instances of the app, and prints the output.

If you use the '--once' flag tsuru will run the command only in one unit.

If you don't provide the app name, tsuru will try to guess it.
`
	return &cmd.Info{
		Name:    "run",
		Usage:   `run <command> [commandarg1] [commandarg2] ... [commandargn] [--app appname] [--once]`,
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *AppRun) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/run?once=%t", appName, c.once))
	if err != nil {
		return err
	}
	b := strings.NewReader(strings.Join(context.Args, " "))
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.Copy(context.Stdout, r.Body)
	return err
}

func (c *AppRun) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.GuessingCommand.Flags()
		c.fs.BoolVar(&c.once, "once", false, "Running only one unit")
		c.fs.BoolVar(&c.once, "o", false, "Running only one unit")
	}
	return c.fs
}
