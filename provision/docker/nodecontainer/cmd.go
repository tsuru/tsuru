// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

const (
	emptyPoolLabel = "<all>"
)

type NodeContainerList struct {
	fs        *gnuflag.FlagSet
	namesOnly bool
}

func (c *NodeContainerList) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "node-container-list",
		Usage: "node-container-list",
		Desc:  "List all existing node containers.",
	}
}

func (c *NodeContainerList) Run(context *cmd.Context, client *cmd.Client) error {
	u, err := cmd.GetURL("/docker/nodecontainers")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	rsp, err := client.Do(request)
	if err != nil {
		return err
	}
	var all []NodeContainerConfigGroup
	err = json.NewDecoder(rsp.Body).Decode(&all)
	if err != nil {
		return err
	}
	if c.namesOnly {
		for _, entry := range all {
			fmt.Fprintln(context.Stdout, entry.Name)
		}
		return nil
	}
	tbl := cmd.NewTable()
	tbl.LineSeparator = true
	tbl.Headers = cmd.Row{"Name", "Pool Configs", "Image"}
	for _, entry := range all {
		var pools []string
		for poolName := range entry.ConfigPools {
			if poolName == "" {
				poolName = emptyPoolLabel
			}
			pools = append(pools, poolName)
		}
		sort.Strings(pools)
		var images []string
		for _, p := range pools {
			if p == emptyPoolLabel {
				p = ""
			}
			poolEntry := entry.ConfigPools[p]
			images = append(images, poolEntry.image())
		}
		tbl.AddRow(cmd.Row{entry.Name, strings.Join(pools, "\n"), strings.Join(images, "\n")})
	}
	tbl.Sort()
	fmt.Fprint(context.Stdout, tbl.String())
	return nil
}

func (c *NodeContainerList) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("flags", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.namesOnly, "q", false, "Show only names of existing node containers.")
	}
	return c.fs
}

type NodeContainerAdd struct {
	fs   *gnuflag.FlagSet
	raw  cmd.MapFlag
	pool string
}

func (c *NodeContainerAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "node-container-add",
		Usage: "node-container-add <name> [-p/--pool poolname] [-r/--raw path=value]...",
		Desc: `Add new node container or overwrite existing one. If the pool name is omitted
the node container will be valid for all pools.`,
		MinArgs: 1,
		MaxArgs: 1,
	}
}

func (c *NodeContainerAdd) Run(context *cmd.Context, client *cmd.Client) error {
	u, err := cmd.GetURL("/docker/nodecontainers")
	if err != nil {
		return err
	}
	val := url.Values{}
	for k, v := range c.raw {
		val.Set(k, v)
	}
	val.Set("name", context.Args[0])
	val.Set("pool", c.pool)
	reader := strings.NewReader(val.Encode())
	request, err := http.NewRequest("POST", u, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Node container successfully added.")
	return nil
}

func (c *NodeContainerAdd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("flags", gnuflag.ExitOnError)
		msg := "Add raw parameter to node container api call."
		c.fs.Var(&c.raw, "r", msg)
		c.fs.Var(&c.raw, "raw", msg)
		msg = "Pool to add container config. If empty it'll be a default entry to all pools."
		c.fs.StringVar(&c.pool, "p", "", msg)
		c.fs.StringVar(&c.pool, "pool", "", msg)
	}
	return c.fs
}

type NodeContainerInfo struct{}

func (c *NodeContainerInfo) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "node-container-info",
		Usage:   "node-container-info <name>",
		Desc:    "Show details about a single node container.",
		MinArgs: 1,
		MaxArgs: 1,
	}
}

func (c *NodeContainerInfo) Run(context *cmd.Context, client *cmd.Client) error {
	u, err := cmd.GetURL("/docker/nodecontainers/" + context.Args[0])
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	rsp, err := client.Do(request)
	if err != nil {
		return err
	}
	var poolConfigs map[string]NodeContainerConfig
	err = json.NewDecoder(rsp.Body).Decode(&poolConfigs)
	if err != nil {
		return err
	}
	tbl := cmd.NewTable()
	tbl.LineSeparator = true
	tbl.Headers = cmd.Row{"Pool", "Config"}
	for poolName, config := range poolConfigs {
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		if poolName == "" {
			poolName = emptyPoolLabel
		}
		tbl.AddRow(cmd.Row{poolName, string(data)})
	}
	tbl.Sort()
	fmt.Fprint(context.Stdout, tbl.String())
	return nil
}

type NodeContainerUpdate struct {
	fs   *gnuflag.FlagSet
	raw  cmd.MapFlag
	pool string
}

func (c *NodeContainerUpdate) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "node-container-update",
		Usage: "node-container-update <name> [-p/--pool poolname] [-r/--raw path=value]...",
		Desc: `Update an existing node container. If the pool name is omitted the default
configuration will be updated. When updating node containers the specified
configuration will be merged with the existing configuration.`,
		MinArgs: 1,
		MaxArgs: 1,
	}
}

func (c *NodeContainerUpdate) Run(context *cmd.Context, client *cmd.Client) error {
	u, err := cmd.GetURL("/docker/nodecontainers/" + context.Args[0])
	if err != nil {
		return err
	}
	val := url.Values{}
	for k, v := range c.raw {
		val.Set(k, v)
	}
	val.Set("pool", c.pool)
	reader := strings.NewReader(val.Encode())
	request, err := http.NewRequest("POST", u, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Node container successfully updated.")
	return nil
}

func (c *NodeContainerUpdate) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("flags", gnuflag.ExitOnError)
		msg := "Add raw parameter to node container api call."
		c.fs.Var(&c.raw, "r", msg)
		c.fs.Var(&c.raw, "raw", msg)
		msg = "Pool to update container config. If empty it'll be a default entry to all pools."
		c.fs.StringVar(&c.pool, "p", "", msg)
		c.fs.StringVar(&c.pool, "pool", "", msg)
	}
	return c.fs
}

type NodeContainerDelete struct {
	cmd.ConfirmationCommand
	fs   *gnuflag.FlagSet
	pool string
}

func (c *NodeContainerDelete) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "node-container-delete",
		Usage:   "node-container-delete <name> [-p/--pool poolname] [-y]",
		Desc:    "Delete existing node container.",
		MinArgs: 1,
		MaxArgs: 1,
	}
}

func (c *NodeContainerDelete) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	if !c.Confirm(context, "Are you sure you want to remove node container?") {
		return nil
	}
	val := url.Values{}
	val.Set("pool", c.pool)
	u, err := cmd.GetURL(fmt.Sprintf("/docker/nodecontainers/%s?%s", context.Args[0], val.Encode()))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Node container successfully deleted.")
	return nil
}

func (c *NodeContainerDelete) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.ConfirmationCommand.Flags()
		msg := "Pool to remove container config. If empty the default node container will be removed."
		c.fs.StringVar(&c.pool, "p", "", msg)
		c.fs.StringVar(&c.pool, "pool", "", msg)
	}
	return c.fs
}

type NodeContainerUpgrade struct {
	cmd.ConfirmationCommand
}

func (c *NodeContainerUpgrade) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "node-container-upgrade",
		Usage:   "node-container-upgrade <name> [-p/--pool poolname] [-y]",
		Desc:    "Upgrade version and restart node containers.",
		MinArgs: 1,
		MaxArgs: 1,
	}
}

func (c *NodeContainerUpgrade) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	if !c.Confirm(context, "Are you sure you want to upgrade existing node containers?") {
		return nil
	}
	u, err := cmd.GetURL(fmt.Sprintf("/docker/nodecontainers/%s/upgrade", context.Args[0]))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return err
	}
	rsp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	return cmd.StreamJSONResponse(context.Stdout, rsp)
}
