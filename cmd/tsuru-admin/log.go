// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "github.com/globocom/tsuru/cmd"

type LogRemove struct{}

func (c *LogRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "log-remove",
		Usage:   "log-remove",
		Desc:    `remove all app logs.`,
		MinArgs: 0,
	}
}
