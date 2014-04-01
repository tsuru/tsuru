// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"github.com/globocom/tsuru/cmd"
	"io/ioutil"
	"net/http"
	"os"
)

type pluginInstall struct{}

func (pluginInstall) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "plugin-install",
		Usage:   "plugin-install <plugin-name> <plugin-url>",
		Desc:    "Install tsuru plugins.",
		MinArgs: 2,
	}
}

func (c *pluginInstall) Run(context *cmd.Context, client *cmd.Client) error {
	pluginsDir := cmd.JoinWithUserDir(".tsuru", "plugins")
	err := filesystem().MkdirAll(pluginsDir, 0755)
	if err != nil {
		return err
	}
	pluginName := context.Args[0]
	pluginUrl := context.Args[1]
	pluginPath := cmd.JoinWithUserDir(".tsuru", "plugins", pluginName)
	file, err := filesystem().OpenFile(pluginPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	resp, err := http.Get(pluginUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.New("Failed to install plugin.")
	}
	return nil
}
