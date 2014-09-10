// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/tsuru/tsuru/cmd"
)

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

func (c *AppInfo) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s", appName))
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
	url, err = cmd.GetURL(fmt.Sprintf("/docker/node/apps/%s/containers", appName))
	if err != nil {
		return err
	}
	request, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	response, err = client.Do(request)
	var adminResult []byte
	if err == nil {
		defer response.Body.Close()
		adminResult, err = ioutil.ReadAll(response.Body)
	}
	return c.Show(result, adminResult, context)
}

type unit struct {
	Name   string
	Ip     string
	Status string
}

func (u *unit) Available() bool {
	return u.Status == "started" || u.Status == "unreachable"
}

type app struct {
	Ip         string
	CName      []string
	Name       string
	Platform   string
	Repository string
	Teams      []string
	Units      []unit
	Ready      bool
	Owner      string
	TeamOwner  string
	Deploys    uint
	containers []container
}

type container struct {
	ID               string
	Type             string
	IP               string
	HostAddr         string
	HostPort         string
	SSHHostPort      string
	Status           string
	Version          string
	Image            string
	LastStatusUpdate time.Time
}

func (a *app) Addr() string {
	cnames := strings.Join(a.CName, ", ")
	if cnames != "" {
		return fmt.Sprintf("%s, %s", cnames, a.Ip)
	}
	return a.Ip
}

func (a *app) IsReady() string {
	if a.Ready {
		return "Yes"
	}
	return "No"
}

func (a *app) GetTeams() string {
	return strings.Join(a.Teams, ", ")
}

func (a *app) String() string {
	format := `Application: {{.Name}}
Repository: {{.Repository}}
Platform: {{.Platform}}
Teams: {{.GetTeams}}
Address: {{.Addr}}
Owner: {{.Owner}}
Team owner: {{.TeamOwner}}
Deploys: {{.Deploys}}
`
	tmpl := template.Must(template.New("app").Parse(format))
	units := cmd.NewTable()
	titles := []string{"Unit", "State"}
	contMap := map[string]container{}
	if len(a.containers) > 0 {
		for _, cont := range a.containers {
			id := cont.ID
			if len(cont.ID) > 10 {
				id = id[:10]
			}
			contMap[id] = cont
		}
		titles = append(titles, []string{"Host", "Port", "IP"}...)
	}
	units.Headers = cmd.Row(titles)
	for _, unit := range a.Units {
		if unit.Name != "" {
			id := unit.Name
			if len(unit.Name) > 10 {
				id = id[:10]
			}
			row := []string{id, unit.Status}
			cont, ok := contMap[id]
			if ok {
				row = append(row, []string{cont.HostAddr, cont.HostPort, cont.IP}...)
			}
			units.AddRow(cmd.Row(row))
		}
	}
	if len(a.containers) > 0 {
		units.SortByColumn(2)
	}
	var buf bytes.Buffer
	tmpl.Execute(&buf, a)
	var suffix string
	if units.Rows() > 0 {
		suffix = fmt.Sprintf("Units:\n%s", units)
	}
	return buf.String() + suffix
}

func (c *AppInfo) Show(result []byte, adminResult []byte, context *cmd.Context) error {
	var a app
	err := json.Unmarshal(result, &a)
	if err != nil {
		return err
	}
	json.Unmarshal(adminResult, &a.containers)
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

func (c *AppGrant) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	teamName := context.Args[0]
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/teams/%s", appName, teamName))
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

func (c *AppRevoke) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	teamName := context.Args[0]
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/teams/%s", appName, teamName))
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

func (c AppList) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/apps")
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
	table.Headers = cmd.Row([]string{"Application", "Units State Summary", "Address", "Ready?"})
	for _, app := range apps {
		var available int
		var total int
		for _, unit := range app.Units {
			if unit.Name != "" {
				total++
				if unit.Available() {
					available += 1
				}
			}
		}
		summary := fmt.Sprintf("%d of %d units in-service", available, total)
		addrs := strings.Replace(app.Addr(), ", ", "\n", -1)
		table.AddRow(cmd.Row([]string{app.Name, summary, addrs, app.IsReady()}))
	}
	table.LineSeparator = true
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

type AppStop struct {
	GuessingCommand
}

func (c *AppStop) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "stop",
		Usage: "stop [--app appname]",
		Desc: `stops an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *AppStop) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/stop", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, nil)
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

type AppStart struct {
	GuessingCommand
}

func (c *AppStart) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "start",
		Usage: "start [--app appname]",
		Desc: `starts an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *AppStart) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/start", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, nil)
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

type AppRestart struct {
	GuessingCommand
}

func (c *AppRestart) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/restart", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, nil)
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

type AddCName struct {
	GuessingCommand
}

func (c *AddCName) Run(context *cmd.Context, client *cmd.Client) error {
	err := addCName(context.Args, c.GuessingCommand, client)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "cname successfully defined.")
	return nil
}

func (c *AddCName) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-cname",
		Usage:   "add-cname <cname> [<cname> ...] [--app appname]",
		Desc:    `adds a cname for your app.`,
		MinArgs: 1,
	}
}

type RemoveCName struct {
	GuessingCommand
}

func (c *RemoveCName) Run(context *cmd.Context, client *cmd.Client) error {
	err := unsetCName(context.Args, c.GuessingCommand, client)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "cname successfully undefined.")
	return nil
}

func (c *RemoveCName) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-cname",
		Usage:   "remove-cname <cname> [<cname> ...] [--app appname]",
		Desc:    `removes cnames of your app.`,
		MinArgs: 1,
	}
}

func unsetCName(v []string, g GuessingCommand, client *cmd.Client) error {
	appName, err := g.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/cname", appName))
	if err != nil {
		return err
	}
	cnames := make(map[string][]string)
	cnames["cname"] = v
	c, err := json.Marshal(cnames)
	if err != nil {
		return err
	}
	body := bytes.NewReader(c)
	request, err := http.NewRequest("DELETE", url, body)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	return nil
}

func addCName(v []string, g GuessingCommand, client *cmd.Client) error {
	appName, err := g.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/cname", appName))
	if err != nil {
		return err
	}
	cnames := make(map[string][]string)
	cnames["cname"] = v
	c, err := json.Marshal(cnames)
	if err != nil {
		return err
	}
	body := bytes.NewReader(c)
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
