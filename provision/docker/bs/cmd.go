// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/provision"
)

type EnvSetCmd struct {
	fs   *gnuflag.FlagSet
	pool string
}

func (c *EnvSetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bs-env-set",
		Usage:   "bs-env-set <NAME=value> [NAME=value]... [-p/--pool poolname]",
		Desc:    "Set environment variables used when starting bs (big sibling) container.",
		MinArgs: 1,
	}
}

func (c *EnvSetCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	url, err := cmd.GetURL("/docker/bs/env")
	if err != nil {
		return err
	}
	var envList []provision.Entry
	for _, arg := range context.Args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid variable values")
		}
		if parts[0] == "" {
			return fmt.Errorf("invalid variable values")
		}
		envList = append(envList, provision.Entry{Name: parts[0], Value: parts[1]})
	}
	conf := provision.ScopedConfig{}
	if c.pool == "" {
		conf.Envs = envList
	} else {
		conf.Pools = []provision.PoolEntry{{
			Name: c.pool,
			Envs: envList,
		}}
	}
	b, err := json.Marshal(conf)
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
	return cmd.StreamJSONResponse(context.Stdout, response)
}

func (c *EnvSetCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		desc := "Pool name where set variables will apply"
		c.fs.StringVar(&c.pool, "pool", "", desc)
		c.fs.StringVar(&c.pool, "p", "", desc)
	}
	return c.fs
}

type InfoCmd struct{}

func (c *InfoCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bs-info",
		Usage:   "bs-info",
		Desc:    "Prints information about bs (big sibling) containers.",
		MinArgs: 0,
	}
}

func (c *InfoCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/bs")
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
	var conf provision.ScopedConfig
	err = json.NewDecoder(response.Body).Decode(&conf)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Image: %s\n\nEnvironment Variables [Default]:\n", conf.Extra["image"])
	t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
	for _, envVar := range conf.Envs {
		t.AddRow(cmd.Row([]string{envVar.Name, fmt.Sprintf("%v", envVar.Value)}))
	}
	context.Stdout.Write(t.Bytes())
	for _, pool := range conf.Pools {
		t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
		fmt.Fprintf(context.Stdout, "\nEnvironment Variables [%s]:\n", pool.Name)
		for _, envVar := range pool.Envs {
			t.AddRow(cmd.Row([]string{envVar.Name, fmt.Sprintf("%v", envVar.Value)}))
		}
		context.Stdout.Write(t.Bytes())
	}
	return nil
}

type UpgradeCmd struct{}

func (c *UpgradeCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bs-upgrade",
		Usage:   "bs-upgrade",
		Desc:    "Upgrade the version of bs containers.",
		MinArgs: 0,
	}
}

func (c *UpgradeCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	url, err := cmd.GetURL("/docker/bs/upgrade")
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
	return cmd.StreamJSONResponse(context.Stdout, response)
}
