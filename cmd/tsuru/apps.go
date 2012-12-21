// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
	"strings"
)

var AppName = gnuflag.String("app", "", "App name for running app related commands.")
var AssumeYes = gnuflag.Bool("assume-yes", false, "Don't ask for confirmation on operations.")
var LogLines = gnuflag.Int("lines", 10, "The number of log lines to display")
var LogSource = gnuflag.String("source", "", "The log from the given source")

type AppInfo struct {
	GuessingCommand
}

func (c *AppInfo) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-info",
		Usage: "app-info [--app appname]",
		Desc: `show information about your app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *AppInfo) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s", appName))
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return c.Show(result, context)
}

type unit struct {
	Name  string
	Ip    string
	State string
}

type app struct {
	Name       string
	Framework  string
	Repository string
	State      string
	Teams      []string
	Units      []unit
}

func (a *app) String() string {
	format := `Application: %s
State: %s
Repository: %s
Platform: %s
Units: %s
Teams: %s`
	teams := strings.Join(a.Teams, ", ")
	ips := make([]string, len(a.Units))
	for i, unit := range a.Units {
		ips[i] = unit.Ip
	}
	units := strings.Join(ips, ", ")
	return fmt.Sprintf(format, a.Name, a.State, a.Repository, a.Framework, units, teams)
}

func (c *AppInfo) Show(result []byte, context *cmd.Context) error {
	var a app
	err := json.Unmarshal(result, &a)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, &a)
	return nil
}

type AppGrant struct {
	GuessingCommand
}

func (c *AppGrant) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-grant",
		Usage: "app-grant <teamname> [--app appname]",
		Desc: `grants access to an app to a team.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *AppGrant) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	teamName := context.Args[0]
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `Team "%s" was added to the "%s" app`+"\n", teamName, appName)
	return nil
}

type AppRevoke struct {
	GuessingCommand
}

func (c *AppRevoke) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-revoke",
		Usage: "app-revoke <teamname> [--app appname]",
		Desc: `revokes access to an app from a team.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *AppRevoke) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	teamName := context.Args[0]
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `Team "%s" was removed from the "%s" app`+"\n", teamName, appName)
	return nil
}

type AppModel struct {
	Name  string
	State string
	Units []Units
}

type Units struct {
	Ip string
}

type AppList struct{}

func (c *AppList) Run(context *cmd.Context, client cmd.Doer) error {
	request, err := http.NewRequest("GET", cmd.GetUrl("/apps"), nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return c.Show(result, context)
}

func (c *AppList) Show(result []byte, context *cmd.Context) error {
	var apps []AppModel
	err := json.Unmarshal(result, &apps)
	if err != nil {
		return err
	}
	table := cmd.NewTable()
	table.Headers = cmd.Row([]string{"Application", "State", "Ip"})
	for _, app := range apps {
		ip := ""
		if len(app.Units) > 0 {
			ip = app.Units[0].Ip
		}
		table.AddRow(cmd.Row([]string{app.Name, app.State, ip}))
	}
	context.Stdout.Write(table.Bytes())
	return nil
}

func (c *AppList) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-list",
		Usage: "app-list",
		Desc:  "list all your apps.",
	}
}

type AppRestart struct {
	GuessingCommand
}

func (c *AppRestart) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/restart", appName))
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.Copy(context.Stdout, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func (c *AppRestart) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "restart",
		Usage: "restart [--app appname]",
		Desc: `restarts an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}
