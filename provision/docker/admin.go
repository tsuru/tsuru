// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
)

type moveContainersCmd struct{}

type progressFormatter struct{}

func (progressFormatter) Format(out io.Writer, data []byte) error {
	var logEntry progressLog
	err := json.Unmarshal(data, &logEntry)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", logEntry.Message)
	return nil
}

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
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
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
	defer response.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
}

type rebalanceContainersCmd struct {
	fs  *gnuflag.FlagSet
	dry bool
}

func (c *rebalanceContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "containers-rebalance",
		Usage:   "containers-rebalance [--dry]",
		Desc:    "Move containers creating a more even distribution between docker nodes.",
		MinArgs: 0,
	}
}

func (c *rebalanceContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/containers/rebalance")
	if err != nil {
		return err
	}
	params := map[string]string{
		"dry": fmt.Sprintf("%t", c.dry),
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
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
}

func (c *rebalanceContainersCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("containers-rebalance", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.dry, "dry", false, "Dry run, only shows what would be done")
	}
	return c.fs
}
