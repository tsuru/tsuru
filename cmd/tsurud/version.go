// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/cmd"
)

type versionCmd struct {
}

func (c *versionCmd) Run(context *cmd.Context) error {
	fmt.Printf("tsurud version %s (git commit %s)\n", api.Version, api.GitHash)
	return nil
}

func (versionCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "version",
		Usage:   "version",
		Desc:    "Show tsuru version",
		MinArgs: 0,
	}
}
