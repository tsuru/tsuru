// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ajg/form"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

type moveContainersCmd struct{}

func (c *moveContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "containers-move",
		Usage: "containers-move <from host> <to host>",
		Desc: `Move all containers from one host to another.
This command allows you to move all containers from one host to another. This
is useful when doing maintenance on hosts. <from host> and <to host> must be
host names of existing docker nodes.

This command will go through the following steps:

* Enumerate all units at the origin host;
* For each unit, create a new unit at the destination host;
* Erase each unit from the origin host.`,
		MinArgs: 2,
	}
}

func (c *moveContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	u, err := cmd.GetURL("/docker/containers/move")
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Set("from", context.Args[0])
	v.Set("to", context.Args[1])
	request, err := http.NewRequest("POST", u, bytes.NewBufferString(v.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return cmd.StreamJSONResponse(context.Stdout, response)
}

type moveContainerCmd struct{}

func (c *moveContainerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "container-move",
		Usage: "container-move <container id> <to host>",
		Desc: `Move specified container to another host.
This command allow you to specify a container id and a destination host, this
will create a new container on the destination host and remove the container
from its previous host.`,
		MinArgs: 2,
	}
}

func (c *moveContainerCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	u, err := cmd.GetURL(fmt.Sprintf("/docker/container/%s/move", context.Args[0]))
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Set("to", context.Args[1])
	request, err := http.NewRequest("POST", u, bytes.NewBufferString(v.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return cmd.StreamJSONResponse(context.Stdout, response)
}

type rebalanceContainersCmd struct {
	cmd.ConfirmationCommand
	fs             *gnuflag.FlagSet
	dry            bool
	metadataFilter cmd.MapFlag
	appFilter      cmd.StringSliceFlag
}

func (c *rebalanceContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "containers-rebalance",
		Usage: "containers-rebalance [--dry] [-y/--assume-yes] [-m/--metadata <metadata>=<value>]... [-a/--app <appname>]...",
		Desc: `Move containers creating a more even distribution between docker nodes.
Instead of specifying hosts as in the containers-move command, this command
will automatically choose to which host each unit should be moved, trying to
distribute the units as evenly as possible.

The --dry flag runs the balancing algorithm without doing any real
modification. It will only print which units would be moved and where they
would be created.`,
		MinArgs: 0,
	}
}

func (c *rebalanceContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	if !c.dry && !c.Confirm(context, "Are you sure you want to rebalance containers?") {
		return nil
	}
	u, err := cmd.GetURL("/docker/containers/rebalance")
	if err != nil {
		return err
	}
	opts := rebalanceOptions{
		Dry: c.dry,
	}
	if len(c.metadataFilter) > 0 {
		opts.MetadataFilter = c.metadataFilter
	}
	if len(c.appFilter) > 0 {
		opts.AppFilter = c.appFilter
	}
	v, err := form.EncodeToValues(&opts)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", u, bytes.NewBufferString(v.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return cmd.StreamJSONResponse(context.Stdout, response)
}

func (c *rebalanceContainersCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.ConfirmationCommand.Flags()
		c.fs.BoolVar(&c.dry, "dry", false, "Dry run, only shows what would be done")
		desc := "Filter by host metadata"
		c.fs.Var(&c.metadataFilter, "metadata", desc)
		c.fs.Var(&c.metadataFilter, "m", desc)
		desc = "Filter by app name"
		c.fs.Var(&c.appFilter, "app", desc)
		c.fs.Var(&c.appFilter, "a", desc)
	}
	return c.fs
}
