// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/api"
	"github.com/globocom/tsuru/cmd"
)

type apiCmd struct{}

func (apiCmd) Run(context *cmd.Context, client *cmd.Client) error {
	api.RunServer()
	return nil
}

func (apiCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api",
		Usage:   "api",
		Desc:    "Starts the tsuru api webserver.",
		MinArgs: 0,
	}
}
