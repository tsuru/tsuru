// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"fmt"

	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gnuflag"
)

type ConfirmationCommand struct {
	yes bool
	fs  *gnuflag.FlagSet
}

func (cmd *ConfirmationCommand) Flags() *gnuflag.FlagSet {
	if cmd.fs == nil {
		cmd.fs = gnuflag.NewFlagSet("", gnuflag.ExitOnError)
		cmd.fs.BoolVar(&cmd.yes, "y", false, "Don't ask for confirmation.")
		cmd.fs.BoolVar(&cmd.yes, "assume-yes", false, "Don't ask for confirmation.")
	}
	return cmd.fs
}

func (cmd *ConfirmationCommand) Confirm(context *cmd.Context, question string) bool {
	if cmd.yes {
		return true
	}
	fmt.Fprintf(context.Stdout, `%s (y/n) `, question)
	var answer string
	fmt.Fscanf(context.Stdin, "%s", &answer)
	if answer != "y" {
		fmt.Fprintln(context.Stdout, "Abort.")
		return false
	}
	return true
}
