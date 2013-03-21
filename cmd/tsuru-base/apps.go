// Copyright 2013 tsuru authors. All rights reserved.
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
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s", appName))
	if err != nil {
		return err
	}
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
	Ip         string
	CName      string
	Name       string
	Framework  string
	Repository string
	Teams      []string
	Units      []unit
}

func (a *app) Addr() string {
	if a.CName != "" {
		return a.CName
	}
	return a.Ip
}

func (a *app) String() string {
	format := `Application: %s
Repository: %s
Platform: %s
Teams: %s
Address: %s
`
	teams := strings.Join(a.Teams, ", ")
	units := cmd.NewTable()
	units.Headers = cmd.Row([]string{"Unit", "State"})
	for _, unit := range a.Units {
		units.AddRow(cmd.Row([]string{unit.Name, unit.State}))
	}
	args := []interface{}{a.Name, a.Repository, a.Framework, teams, a.Addr()}
	if len(a.Units) > 0 {
		format += "Units:\n%s"
		args = append(args, units)
	}
	return fmt.Sprintf(format, args...)
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
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
	if err != nil {
		return err
	}
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
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
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
	fmt.Fprintf(context.Stdout, `Team "%s" was removed from the "%s" app`+"\n", teamName, appName)
	return nil
}

type AppList struct{}

func (c AppList) Run(context *cmd.Context, client cmd.Doer) error {
	url, err := cmd.GetUrl("/apps")
	if err != nil {
		return err
	}
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

func (c AppList) Show(result []byte, context *cmd.Context) error {
	var apps []app
	err := json.Unmarshal(result, &apps)
	if err != nil {
		return err
	}
	table := cmd.NewTable()
	table.Headers = cmd.Row([]string{"Application", "Units State Summary", "Address"})
	for _, app := range apps {
		var units_started int
		for _, unit := range app.Units {
			if unit.State == "started" {
				units_started += 1
			}
		}
		summary := fmt.Sprintf("%d of %d units in-service", units_started, len(app.Units))
		table.AddRow(cmd.Row([]string{app.Name, summary, app.Addr()}))
	}
	table.Sort()
	context.Stdout.Write(table.Bytes())
	return nil
}

func (c AppList) Info() *cmd.Info {
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
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s/restart", appName))
	if err != nil {
		return err
	}
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

type SetCName struct {
	GuessingCommand
}

func (c *SetCName) Run(context *cmd.Context, client cmd.Doer) error {
	err := setCName(context.Args[0], c.GuessingCommand, client)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "cname successfully defined.")
	return nil
}

func (c *SetCName) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-cname",
		Usage:   "set-cname <cname> [--app appname]",
		Desc:    `defines a cname for your app.`,
		MinArgs: 1,
	}
}

type UnsetCName struct {
	GuessingCommand
}

func (c *UnsetCName) Run(context *cmd.Context, client cmd.Doer) error {
	err := setCName("", c.GuessingCommand, client)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "cname successfully undefined.")
	return nil
}

func (c *UnsetCName) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset-cname",
		Usage:   "unset-cname [--app appname]",
		Desc:    `unsets the current cname of your app.`,
		MinArgs: 0,
	}
}

func setCName(v string, g GuessingCommand, client cmd.Doer) error {
	appName, err := g.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetUrl(fmt.Sprintf("/apps/%s", appName))
	if err != nil {
		return err
	}
	body := strings.NewReader(fmt.Sprintf(`{"cname": "%s"}`, v))
	request, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	return nil
}
