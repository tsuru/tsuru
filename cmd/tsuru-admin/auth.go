// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gnuflag"
	"net/http"
	"strings"
)

type tokenGen struct {
	export bool
}

func (c *tokenGen) Run(ctx *cmd.Context, client *cmd.Client) error {
	app := ctx.Args[0]
	url, err := cmd.GetURL("/tokens")
	if err != nil {
		return err
	}
	body := strings.NewReader(fmt.Sprintf(`{"client":"%s","export":%v}`, app, c.export))
	request, _ := http.NewRequest("POST", url, body)
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var token map[string]string
	err = json.NewDecoder(resp.Body).Decode(&token)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "Application token: %q.\n", token["token"])
	return nil
}

func (c *tokenGen) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "token-gen",
		MinArgs: 1,
		Usage:   "token-gen <app-name>",
		Desc:    "Generates an authentication token for an app.",
	}
}

func (c *tokenGen) Flags() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("token-gen", gnuflag.ExitOnError)
	fs.BoolVar(&c.export, "export", false, "Define the token as environment variable in the app")
	fs.BoolVar(&c.export, "e", false, "Define the token as environment variable in the app")
	return fs
}
