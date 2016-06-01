// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

type ListHealingHistoryCmd struct {
	fs            *gnuflag.FlagSet
	nodeOnly      bool
	containerOnly bool
}

func (c *ListHealingHistoryCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
}

func renderHistoryTable(history []HealingEvent, filter string, ctx *cmd.Context) {
	fmt.Fprintln(ctx.Stdout, strings.ToUpper(filter[:1])+filter[1:]+":")
	headers := cmd.Row([]string{"Start", "Finish", "Success", "Failing", "Created", "Error"})
	t := cmd.Table{Headers: headers}
	for i := len(history) - 1; i >= 0; i-- {
		event := history[i]
		if event.Action != filter+"-healing" {
			continue
		}
		data := make([]string, 2)
		if filter == "node" {
			data[0] = event.FailingNode.Address
			data[1] = event.CreatedNode.Address
		} else {
			data[0] = event.FailingContainer.ID
			data[1] = event.CreatedContainer.ID
			if len(data[0]) > 10 {
				data[0] = data[0][:10]
			}
			if len(data[1]) > 10 {
				data[1] = data[1][:10]
			}
		}
		t.AddRow(cmd.Row([]string{
			event.StartTime.Local().Format(time.Stamp),
			event.EndTime.Local().Format(time.Stamp),
			fmt.Sprintf("%t", event.Successful),
			data[0],
			data[1],
			event.Error,
		}))
	}
	t.LineSeparator = true
	t.Reverse()
	ctx.Stdout.Write(t.Bytes())
}

func (c *ListHealingHistoryCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var filter string
	if c.nodeOnly && !c.containerOnly {
		filter = "node"
	}
	if c.containerOnly && !c.nodeOnly {
		filter = "container"
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/healing?filter=%s", filter))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var history []HealingEvent
	if resp.StatusCode == http.StatusOK {
		err = json.NewDecoder(resp.Body).Decode(&history)
		if err != nil {
			return err
		}
	}
	if filter != "" {
		renderHistoryTable(history, filter, ctx)
	} else {
		renderHistoryTable(history, "node", ctx)
		renderHistoryTable(history, "container", ctx)
	}
	return nil
}

func (c *ListHealingHistoryCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.BoolVar(&c.nodeOnly, "node", false, "List only healing process started for nodes")
		c.fs.BoolVar(&c.containerOnly, "container", false, "List only healing process started for containers")
	}
	return c.fs
}

type GetNodeHealingConfigCmd struct{}

func (c *GetNodeHealingConfigCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-info",
		Usage: "docker-healing-info",
		Desc:  "Show the current configuration for active healing nodes.",
	}
}

func (c *GetNodeHealingConfigCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/healing/node")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var conf map[string]NodeHealerConfig
	err = json.NewDecoder(resp.Body).Decode(&conf)
	if err != nil {
		return err
	}
	v := func(v *int) string {
		if v == nil || *v == 0 {
			return "disabled"
		}
		return fmt.Sprintf("%ds", *v)
	}
	baseConf := conf[""]
	delete(conf, "")
	fmt.Fprint(ctx.Stdout, "Default:\n")
	tbl := cmd.NewTable()
	tbl.Headers = cmd.Row{"Config", "Value"}
	tbl.AddRow(cmd.Row{"Enabled", fmt.Sprintf("%v", baseConf.Enabled != nil && *baseConf.Enabled)})
	tbl.AddRow(cmd.Row{"Max unresponsive time", v(baseConf.MaxUnresponsiveTime)})
	tbl.AddRow(cmd.Row{"Max time since success", v(baseConf.MaxTimeSinceSuccess)})
	fmt.Fprint(ctx.Stdout, tbl.String())
	if len(conf) > 0 {
		fmt.Fprintln(ctx.Stdout)
	}
	poolNames := make([]string, 0, len(conf))
	for pool := range conf {
		poolNames = append(poolNames, pool)
	}
	sort.Strings(poolNames)
	for i, name := range poolNames {
		poolConf := conf[name]
		fmt.Fprintf(ctx.Stdout, "Pool %q:\n", name)
		tbl := cmd.NewTable()
		tbl.Headers = cmd.Row{"Config", "Value", "Inherited"}
		tbl.AddRow(cmd.Row{"Enabled", fmt.Sprintf("%v", poolConf.Enabled != nil && *poolConf.Enabled), strconv.FormatBool(poolConf.EnabledInherited)})
		tbl.AddRow(cmd.Row{"Max unresponsive time", v(poolConf.MaxUnresponsiveTime), strconv.FormatBool(poolConf.MaxUnresponsiveTimeInherited)})
		tbl.AddRow(cmd.Row{"Max time since success", v(poolConf.MaxTimeSinceSuccess), strconv.FormatBool(poolConf.MaxTimeSinceSuccessInherited)})
		fmt.Fprint(ctx.Stdout, tbl.String())
		if i < len(poolNames)-1 {
			fmt.Fprintln(ctx.Stdout)
		}
	}
	return nil
}

type SetNodeHealingConfigCmd struct {
	fs              *gnuflag.FlagSet
	enable          bool
	disable         bool
	pool            string
	maxUnresponsive int
	maxUnsuccessful int
}

func (c *SetNodeHealingConfigCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-update",
		Usage: "docker-healing-update [-p/--pool pool] [--enable] [--disable] [--max-unresponsive <seconds>] [--max-unsuccessful <seconds>]",
		Desc:  "Update node healing configuration",
	}
}

func (c *SetNodeHealingConfigCmd) Flags() *gnuflag.FlagSet {
	msg := "The pool name to which the configuration will apply. If unset it'll be set as default for all pools."
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.StringVar(&c.pool, "p", "", msg)
		c.fs.StringVar(&c.pool, "pool", "", msg)
		c.fs.BoolVar(&c.enable, "enable", false, "Enable active node healing")
		c.fs.BoolVar(&c.disable, "disable", false, "Disable active node healing")
		c.fs.IntVar(&c.maxUnresponsive, "max-unresponsive", -1, "Number of seconds tsuru will wait for the node to notify it's alive")
		c.fs.IntVar(&c.maxUnsuccessful, "max-unsuccessful", -1, "Number of seconds tsuru will wait for the node to run successul checks")
	}
	return c.fs
}

func (c *SetNodeHealingConfigCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	if c.enable && c.disable {
		return errors.New("conflicting flags --enable and --disable")
	}
	v := url.Values{}
	v.Set("pool", c.pool)
	if c.maxUnresponsive >= 0 {
		v.Set("MaxUnresponsiveTime", strconv.FormatInt(int64(c.maxUnresponsive), 10))
	}
	if c.maxUnsuccessful >= 0 {
		v.Set("MaxTimeSinceSuccess", strconv.FormatInt(int64(c.maxUnsuccessful), 10))
	}
	if c.enable {
		v.Set("Enabled", strconv.FormatBool(true))
	}
	if c.disable {
		v.Set("Enabled", strconv.FormatBool(false))
	}
	body := strings.NewReader(v.Encode())
	u, err := cmd.GetURL("/docker/healing/node")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = client.Do(req)
	if err == nil {
		fmt.Fprintln(ctx.Stdout, "Node healing configuration successfully updated.")
	}
	return err
}

type DeleteNodeHealingConfigCmd struct {
	cmd.ConfirmationCommand
	fs              *gnuflag.FlagSet
	pool            string
	enabled         bool
	maxUnresponsive bool
	maxUnsuccessful bool
}

func (c *DeleteNodeHealingConfigCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-delete",
		Usage: "docker-healing-delete [-p/--pool pool] [--enabled] [--max-unresponsive] [--max-unsuccessful]",
		Desc: `Delete a node healing configuration entry.

If [[--pool]] is provided the configuration entries from the specified pool
will be removed and the default value will be used.

If [[--pool]] is not provided the configuration entry will be removed from the
default configuration.`,
	}
}

func (c *DeleteNodeHealingConfigCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.ConfirmationCommand.Flags()
		msg := "The pool name from where the configuration will be removed. If unset it'll delete the default healing configuration."
		c.fs.StringVar(&c.pool, "p", "", msg)
		c.fs.StringVar(&c.pool, "pool", "", msg)
		c.fs.BoolVar(&c.enabled, "enabled", false, "Remove the 'enabled' configuration option")
		c.fs.BoolVar(&c.maxUnresponsive, "max-unresponsive", false, "Remove the 'max-unresponsive' configuration option")
		c.fs.BoolVar(&c.maxUnsuccessful, "max-unsuccessful", false, "Remove the 'max-unsuccessful' configuration option")
	}
	return c.fs
}

func (c *DeleteNodeHealingConfigCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	msg := "Are you sure you want to remove %snode healing configuration%s?"
	if c.pool == "" {
		msg = fmt.Sprintf(msg, "the default ", "")
	} else {
		msg = fmt.Sprintf(msg, "", " for pool "+c.pool)
	}
	if !c.Confirm(ctx, msg) {
		return errors.New("command aborted by user")
	}
	v := url.Values{}
	v.Set("pool", c.pool)
	if c.enabled {
		v.Add("name", "Enabled")
	}
	if c.maxUnresponsive {
		v.Add("name", "MaxUnresponsiveTime")
	}
	if c.maxUnsuccessful {
		v.Add("name", "MaxTimeSinceSuccess")
	}
	u, err := cmd.GetURL("/docker/healing/node?" + v.Encode())
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err == nil {
		fmt.Fprintln(ctx.Stdout, "Node healing configuration successfully removed.")
	}
	return err
}
