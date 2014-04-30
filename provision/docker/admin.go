// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
)

type moveContainersCmd struct{}

type jsonLogWriter struct {
	w io.Writer
	b []byte
}

func (w *jsonLogWriter) Write(b []byte) (int, error) {
	var logEntry progressLog
	w.b = append(w.b, b...)
	err := json.Unmarshal(w.b, &logEntry)
	if err != nil {
		return len(b), nil
	}
	fmt.Fprintf(w.w, "%s\n", logEntry.Message)
	w.b = nil
	return len(b), nil
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
	url, err := cmd.GetURL("/containers/move")
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
	w := jsonLogWriter{w: context.Stdout}
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(&w, response.Body) {
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
	url, err := cmd.GetURL(fmt.Sprintf("/container/%s/move", context.Args[0]))
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
	w := jsonLogWriter{w: context.Stdout}
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(&w, response.Body) {
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
	url, err := cmd.GetURL("/containers/rebalance")
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
	w := jsonLogWriter{w: context.Stdout}
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(&w, response.Body) {
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
