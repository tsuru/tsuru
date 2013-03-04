// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/cmd/tsuru-base"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
)

var AssumeYes = gnuflag.Bool("assume-yes", false, "Don't ask for confirmation on operations.")
var NumUnits = gnuflag.Uint("units", 1, "How many units should be created with the app.")

type AppCreate struct{}

func (c *AppCreate) Run(context *cmd.Context, client cmd.Doer) error {
	if *NumUnits == 0 {
		return errors.New("Cannot create app with zero units.")
	}
	appName := context.Args[0]
	framework := context.Args[1]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s","framework":"%s","units":%d}`, appName, framework, *NumUnits))
	request, err := http.NewRequest("POST", cmd.GetUrl("/apps"), b)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
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
	var plural string
	if *NumUnits > 1 {
		plural = "s"
	}
	fmt.Fprintf(context.Stdout, "App %q is being created with %d unit%s!\n", appName, *NumUnits, plural)
	fmt.Fprintln(context.Stdout, "Use app-info to check the status of the app and its units.")
	fmt.Fprintf(context.Stdout, "Your repository for %q project is %q\n", appName, out["repository_url"])
	return nil
}

func (c *AppCreate) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <framework> [--units 1]",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
}

type AppRemove struct {
	tsuru.GuessingCommand
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

func (c *AppRemove) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	var answer string
	if !*AssumeYes {
		fmt.Fprintf(context.Stdout, `Are you sure you want to remove app "%s"? (y/n) `, appName)
		fmt.Fscanf(context.Stdin, "%s", &answer)
		if answer != "y" {
			fmt.Fprintln(context.Stdout, "Abort.")
			return nil
		}
	}
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s", appName))
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

func (c *UnitAdd) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/units", appName))
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

func (c *UnitRemove) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/units", appName))
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
