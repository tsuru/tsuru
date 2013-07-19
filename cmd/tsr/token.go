// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/cmd"
)

type tokenCmd struct{}

func (tokenCmd) Run(context *cmd.Context, client *cmd.Client) error {
	t, err := auth.CreateApplicationToken("tsr")
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, t.Token)
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
