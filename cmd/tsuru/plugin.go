// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "github.com/globocom/tsuru/cmd"

type pluginInstal struct{}

func (pluginInstal) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "plugin-install",
		Usage:   "plugin-install",
		Desc:    "Install tsuru plugins.",
		MinArgs: 0,
	}
}

func (c *pluginInstal) Run(context *cmd.Context, client *cmd.Client) error {
	return nil
}
