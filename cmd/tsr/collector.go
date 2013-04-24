// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/cmd"
)

type collectorCmd struct{}

func (collectorCmd) Run(context *cmd.Context, client *cmd.Client) error {
	return nil
}

func (collectorCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "collector",
		Usage:   "collector",
		Desc:    "Starts the tsuru collector.",
		MinArgs: 0,
	}
}
