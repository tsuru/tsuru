// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

type NodeContainerAdd struct {
	fs   *gnuflag.FlagSet
	pool string
}

func (c *NodeContainerAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "node-container-add",
		Usage:   "node-container-add ...",
		Desc:    ``,
		MinArgs: 1,
	}
}

func (c *NodeContainerAdd) Run(context *cmd.Context, client *cmd.Client) error {
	return nil
}

func (c *NodeContainerAdd) Flags() *gnuflag.FlagSet {
	return nil
}
