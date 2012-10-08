// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io"
	"net/http"
	"strings"
)

type AppRun struct{}

func (c *AppRun) Info() *cmd.Info {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	return &cmd.Info{
		Name:    "run",
		Usage:   `run <appname> <command> [commandarg1] [commandarg2] ... [commandargn]`,
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *AppRun) Run(context *cmd.Context, client cmd.Doer) error {
	appName := context.Args[0]
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/run", appName))
	b := strings.NewReader(strings.Join(context.Args[1:], " "))
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
