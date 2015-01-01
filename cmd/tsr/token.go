// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	"github.com/tsuru/tsuru/cmd"
)

type tokenCmd struct{}

func (tokenCmd) Run(context *cmd.Context, client *cmd.Client) error {
	scheme, err := config.GetString("auth:scheme")
	if err != nil {
		scheme = "native"
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	t, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, t.GetValue())
	return nil
}

func (tokenCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "token",
		Usage:   "token",
		Desc:    "Generates a tsuru token.",
		MinArgs: 0,
	}
}
