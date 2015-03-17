// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/repository/gandalf"
)

type gandalfSyncCmd struct{}

func (gandalfSyncCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "gandalf-sync",
		Usage: "gandalf-sync",
		Desc:  "sync users and applications with the configured Gandalf endpoint",
	}
}

func (gandalfSyncCmd) Run(context *cmd.Context, client *cmd.Client) error {
	return gandalf.Sync()
}
