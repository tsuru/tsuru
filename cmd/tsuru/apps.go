// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

type AppInfo struct {
	GuessingCommand
}

func (c *AppInfo) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-info",
		Usage: "app-info [-app appname]",
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

func (c *AppInfo) Show(result []byte, context *cmd.Context) error {
	var app map[string]interface{}
	err := json.Unmarshal(result, &app)
	if err != nil {
		return err
	}
	template := `Application: %s
State: %s
Repository: %s
Platform: %s
Units: %s
Teams: %s
`
	name := app["Name"]
	state := app["State"]
	platform := app["Framework"]
	repository := app["Repository"]
	units := ""
	for _, unit := range app["Units"].([]interface{}) {
		if len(units) > 0 {
			units += ", "
		}
		units += fmt.Sprintf("%s", unit.(map[string]interface{})["Ip"].(string))
	}
	teams := ""
	for _, team := range app["Teams"].([]interface{}) {
		if len(teams) > 0 {
			teams += ", "
		}
		teams += fmt.Sprintf("%s", team.(string))
	}
	out := fmt.Sprintf(template, name, state, repository, platform, units, teams)
	context.Stdout.Write([]byte(out))
	return nil
}

type AppGrant struct {
	GuessingCommand
}

func (c *AppGrant) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-grant",
		Usage: "app-grant <teamname> [-app appname]",
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
		Usage: "app-revoke <teamname> [-app appname]",
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

type AppCreate struct{}

func (c *AppCreate) Run(context *cmd.Context, client cmd.Doer) error {
	appName := context.Args[0]
	framework := context.Args[1]

	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s", "framework":"%s"}`, appName, framework))
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
	fmt.Fprintf(context.Stdout, `App "%s" is being created!`+"\n", appName)
	fmt.Fprint(context.Stdout, "Check its status with app-list.\n")
	fmt.Fprintf(context.Stdout, `Your repository for "%s" project is "%s"`+"\n", appName, out["repository_url"])
	return nil
}

func (c *AppCreate) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "app-create",
		Usage:   "app-create <appname> <framework>",
		Desc:    "create a new app.",
		MinArgs: 2,
	}
}

type AppRemove struct {
	GuessingCommand
}

func (c *AppRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "app-remove",
		Usage: "app-remove [-app appname]",
		Desc: `removes an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *AppRemove) Run(context *cmd.Context, client cmd.Doer) error {
	appName, err := c.Guess()
	var answer string
	fmt.Fprintf(context.Stdout, `Are you sure you want to remove app "%s"? (y/n) `, appName)
	fmt.Fscanf(context.Stdin, "%s", &answer)
	if answer != "y" {
		fmt.Fprintln(context.Stdout, "Abort.")
		return nil
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

type AppLog struct{}

func (c *AppLog) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "log",
		Usage:   "log <appname>",
		Desc:    "show logs for an app.",
		MinArgs: 1,
	}
}

type Log struct {
	Date    time.Time
	Message string
}

func (c *AppLog) Run(context *cmd.Context, client cmd.Doer) error {
	appName := context.Args[0]
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/log", appName))
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
	logs := []Log{}
	err = json.Unmarshal(result, &logs)
	if err != nil {
		return err
	}
	for _, log := range logs {
		context.Stdout.Write([]byte(log.Date.String() + " - " + log.Message + "\n"))
	}
	return err
}

type AppRestart struct{}

func (c *AppRestart) Run(context *cmd.Context, client cmd.Doer) error {
	appName := context.Args[0]
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
		Name:    "restart",
		Usage:   "restart <appname>",
		Desc:    "restarts an app.",
		MinArgs: 1,
	}
}
