// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"net/http"
)

type platform struct {
	Name string
}

type platformList struct{}

func (platformList) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/platforms")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	var platforms []platform
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&platforms)
	if err != nil {
		return err
	}
	if len(platforms) == 0 {
		fmt.Fprintln(context.Stdout, "No platforms available.")
		return nil
	}
	for _, p := range platforms {
		fmt.Fprintf(context.Stdout, "- %s\n", p.Name)
	}
	return nil
}

func (platformList) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "platform-list",
		Usage:   "platform-list",
		Desc:    "Display the list of available platforms.",
		MinArgs: 0,
	}
}
