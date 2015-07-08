// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gnuflag"
)

type bsEnvSetCmd struct {
	fs   *gnuflag.FlagSet
	pool string
}

func (c *bsEnvSetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bs-env-set",
		Usage:   "bs-env-set <NAME=value> [NAME=value]... [-p/--pool poolname]",
		Desc:    "Set environment variables used when starting bs (big sibling) container.",
		MinArgs: 1,
	}
}

func (c *bsEnvSetCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	url, err := cmd.GetURL("/docker/bs/env")
	if err != nil {
		return err
	}
	var envList []bsEnv
	for _, arg := range context.Args {
		parts := strings.SplitN(arg, "=", 2)
		envList = append(envList, bsEnv{Name: parts[0], Value: parts[1]})
	}
	conf := bsConfig{}
	if c.pool == "" {
		conf.Envs = envList
	} else {
		conf.Pools = []bsPoolEnvs{{
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
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Variables successfully set.")
	return nil
}

func (c *bsEnvSetCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		desc := "Pool name where set variables will apply"
		c.fs.StringVar(&c.pool, "pool", "", desc)
		c.fs.StringVar(&c.pool, "p", "", desc)
	}
	return c.fs
}

type bsInfoCmd struct{}

func (c *bsInfoCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bs-info",
		Usage:   "bs-info",
		Desc:    "Prints information about bs (big sibling) containers.",
		MinArgs: 0,
	}
}

func (c *bsInfoCmd) Run(context *cmd.Context, client *cmd.Client) error {
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
	var conf bsConfig
	err = json.NewDecoder(response.Body).Decode(&conf)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Image: %s\n\nEnvironment Variables [Default]:\n", conf.Image)
	t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
	for _, envVar := range conf.Envs {
		t.AddRow(cmd.Row([]string{envVar.Name, envVar.Value}))
	}
	context.Stdout.Write(t.Bytes())
	for _, pool := range conf.Pools {
		t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
		fmt.Fprintf(context.Stdout, "\nEnvironment Variables [%s]:\n", pool.Name)
		for _, envVar := range pool.Envs {
			t.AddRow(cmd.Row([]string{envVar.Name, envVar.Value}))
		}
		context.Stdout.Write(t.Bytes())
	}
	return nil
}
