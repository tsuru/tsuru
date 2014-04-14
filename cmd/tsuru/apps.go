// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
)

type AppCreate struct {
	cmd.Command
	memory int
	swap   int
	fs     *gnuflag.FlagSet
}

func (c *AppCreate) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <platform> [--memory/-m memory_in_mb] [--swap/-s swap_in_mb]",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
}

func (c *AppCreate) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		infoMessage := "The maximum amount of memory reserved to each container for this app"
		c.fs = gnuflag.NewFlagSet("", gnuflag.ExitOnError)
		c.fs.IntVar(&c.memory, "memory", 0, infoMessage)
		c.fs.IntVar(&c.memory, "m", 0, infoMessage)
		infoMessage = "The maximum amount of swap reserved to each container for this app"
		c.fs.IntVar(&c.swap, "swap", 0, infoMessage)
		c.fs.IntVar(&c.swap, "s", 0, infoMessage)
	}
	return c.fs
}

func (c *AppCreate) Run(context *cmd.Context, client *cmd.Client) error {
	appName := context.Args[0]
	platform := context.Args[1]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s","platform":"%s","memory":"%d","swap":"%d"}`, appName, platform, c.memory, c.swap))
	url, err := cmd.GetURL("/apps")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	out := make(map[string]string)
	err = json.Unmarshal(result, &out)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "App %q is being created!\n", appName)
	fmt.Fprintln(context.Stdout, "Use app-info to check the status of the app and its units.")
	fmt.Fprintf(context.Stdout, "Your repository for %q project is %q\n", appName, out["repository_url"])
	return nil
}

type AppRemove struct {
	tsuru.GuessingCommand
	yes bool
	fs  *gnuflag.FlagSet
}

func (c *AppRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-remove",
		Usage: "app-remove [--app appname] [--assume-yes]",
		Desc: `removes an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *AppRemove) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	var answer string
	if !c.yes {
		fmt.Fprintf(context.Stdout, `Are you sure you want to remove app "%s"? (y/n) `, appName)
		fmt.Fscanf(context.Stdin, "%s", &answer)
		if answer != "y" {
			fmt.Fprintln(context.Stdout, "Abort.")
			return nil
		}
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `App "%s" successfully removed!`+"\n", appName)
	return nil
}

func (c *AppRemove) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.GuessingCommand.Flags()
		c.fs.BoolVar(&c.yes, "assume-yes", false, "Don't ask for confirmation, just remove the app.")
		c.fs.BoolVar(&c.yes, "y", false, "Don't ask for confirmation, just remove the app.")
	}
	return c.fs
}

type UnitAdd struct {
	tsuru.GuessingCommand
}

func (c *UnitAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unit-add",
		Usage:   "unit-add <# of units> [--app appname]",
		Desc:    "add new units to an app.",
		MinArgs: 1,
	}
}

func (c *UnitAdd) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/units", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, bytes.NewBufferString(context.Args[0]))
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Units successfully added!")
	return nil
}

type UnitRemove struct {
	tsuru.GuessingCommand
}

func (c *UnitRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unit-remove",
		Usage:   "unit-remove <# of units> [--app appname]",
		Desc:    "remove units from an app.",
		MinArgs: 1,
	}
}

func (c *UnitRemove) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/units", appName))
	if err != nil {
		return err
	}
	body := bytes.NewBufferString(context.Args[0])
	request, err := http.NewRequest("DELETE", url, body)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Units successfully removed!")
	return nil
}
