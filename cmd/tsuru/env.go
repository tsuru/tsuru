// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io/ioutil"
	"net/http"
	"strings"
)

type EnvGet struct {
	GuessingCommand
}

func (c *EnvGet) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-get",
		Usage: "env-get [--app appname] [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...",
		Desc: `retrieve environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *EnvGet) Run(context *cmd.Context, client cmd.Doer) error {
	b, err := requestEnvUrl("GET", c.GuessingCommand, context, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, string(b))
	return nil
}

type EnvSet struct {
	GuessingCommand
}

func (c *EnvSet) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-set",
		Usage: "env-set <NAME=value> [NAME=value] ... [--app appname]",
		Desc: `set environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *EnvSet) Run(context *cmd.Context, client cmd.Doer) error {
	_, err := requestEnvUrl("POST", c.GuessingCommand, context, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully exported\n")
	return nil
}

type EnvUnset struct {
	GuessingCommand
}

func (c *EnvUnset) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-unset",
		Usage: "env-unset <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2] ... [ENVIRONMENT_VARIABLEN] [--app appname]",
		Desc: `unset environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *EnvUnset) Run(context *cmd.Context, client cmd.Doer) error {
	_, err := requestEnvUrl("DELETE", c.GuessingCommand, context, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully unset\n")
	return nil
}

func requestEnvUrl(method string, g GuessingCommand, context *cmd.Context, client cmd.Doer) (string, error) {
	appName, err := g.Guess()
	if err != nil {
		return "", err
	}
	varsStr := strings.Join(context.Args, " ")
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/env", appName))
	body := strings.NewReader(varsStr)
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return "", err
	}
	r, err := client.Do(request, context)
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
