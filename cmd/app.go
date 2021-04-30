// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/pkg/errors"
	"github.com/tsuru/gnuflag"
)

type AppNameMixIn struct {
	fs      *gnuflag.FlagSet
	appName string
}

func (cmd *AppNameMixIn) AppName() (string, error) {
	if cmd.appName == "" {
		return "", errors.Errorf(`The name of the app is required.

Use the --app flag to specify it.

`)
	}
	return cmd.appName, nil
}

func (cmd *AppNameMixIn) Flags() *gnuflag.FlagSet {
	if cmd.fs == nil {
		cmd.fs = gnuflag.NewFlagSet("", gnuflag.ExitOnError)
		cmd.fs.StringVar(&cmd.appName, "app", "", "The name of the app.")
		cmd.fs.StringVar(&cmd.appName, "a", "", "The name of the app.")
	}
	return cmd.fs
}
