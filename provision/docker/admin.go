// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"launchpad.net/gnuflag"
)

type moveContainersCmd struct{}

func (c *moveContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "containers-move",
		Usage:   "containers-move <from host> <to host>",
		Desc:    "Move all containers from one host to another.\nThis command is especially useful for host maintenance.",
		MinArgs: 2,
	}
}

func (c *moveContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/containers/move")
	if err != nil {
		return err
	}
	params := map[string]string{
		"from": context.Args[0],
		"to":   context.Args[1],
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, nil)
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := w.Remaining()
	if len(unparsed) > 0 {
		return fmt.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
}

type fixContainersCmd struct{}

func (fixContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "fix-containers",
		Usage: "fix-containers",
		Desc:  "Fix containers that are broken in the cluster.",
	}
}

func (fixContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/fix-containers")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	return err
}

type moveContainerCmd struct{}

func (c *moveContainerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "container-move",
		Usage:   "container-move <container id> <to host>",
		Desc:    "Move specified container to another host.",
		MinArgs: 2,
	}
}

func (c *moveContainerCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL(fmt.Sprintf("/docker/container/%s/move", context.Args[0]))
	if err != nil {
		return err
	}
	params := map[string]string{
		"to": context.Args[1],
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	w := tsuruIo.NewStreamWriter(context.Stdout, nil)
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := w.Remaining()
	if len(unparsed) > 0 {
		return fmt.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
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
		Name:    "containers-rebalance",
		Usage:   "containers-rebalance [--dry] [-y/--assume-yes] [-m/--metadata <metadata>=<value>]... [-a/--app <appname>]...",
		Desc:    "Move containers creating a more even distribution between docker nodes.",
		MinArgs: 0,
	}
}

func (c *rebalanceContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	if !c.dry && !c.Confirm(context, "Are you sure you want to rebalance containers?") {
		return nil
	}
	url, err := cmd.GetURL("/docker/containers/rebalance")
	if err != nil {
		return err
	}
	params := map[string]interface{}{
		"dry": fmt.Sprintf("%t", c.dry),
	}
	if len(c.metadataFilter) > 0 {
		params["metadataFilter"] = c.metadataFilter
	}
	if len(c.appFilter) > 0 {
		params["appFilter"] = c.appFilter
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	w := tsuruIo.NewStreamWriter(context.Stdout, nil)
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := w.Remaining()
	if len(unparsed) > 0 {
		return fmt.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
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
