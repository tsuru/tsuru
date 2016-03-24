// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cezarsa/form"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

type EnvSetCmd struct {
	fs   *gnuflag.FlagSet
	pool string
}

func (c *EnvSetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "bs-env-set",
		Usage: "bs-env-set <NAME=value> [NAME=value]... [-p/--pool poolname]",
		Desc: `Sets environment variables used when starting bs (big sibling) container.

If the [standard bs image](https://github.com/tsuru/bs) is being used, it's
possible to find which environment variables can be configured in [bs readme
file](https://github.com/tsuru/bs#environment-variables).

If pool name is omited the enviroment variable will apply to all pools, unless
overriden on a specific pool.`,
		MinArgs: 1,
	}
}

func (c *EnvSetCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	url, err := cmd.GetURL("/docker/bs/env")
	if err != nil {
		return err
	}
	conf := BSConfigEntry{
		Envs: map[string]string{},
	}
	for _, arg := range context.Args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid variable values")
		}
		if parts[0] == "" {
			return fmt.Errorf("invalid variable values")
		}
		conf.Envs[parts[0]] = parts[1]
	}
	values, err := form.EncodeToValues(conf)
	if err != nil {
		return err
	}
	values.Set("pool", c.pool)
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		Name:  "bs-info",
		Usage: "bs-info",
		Desc: `Shows information about the bs (big sibling) containers. Includes environment
variables for each pool and docker image being used.`,
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
	var poolEntries map[string]BSConfigEntry
	err = json.NewDecoder(response.Body).Decode(&poolEntries)
	if err != nil {
		return err
	}
	defaultEntry := poolEntries[""]
	delete(poolEntries, "")
	fmt.Fprintf(context.Stdout, "Image: %s\n\nEnvironment Variables [Default]:\n", defaultEntry.Image)
	t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
	for envName, envValue := range defaultEntry.Envs {
		t.AddRow(cmd.Row([]string{envName, fmt.Sprintf("%v", envValue)}))
	}
	t.Sort()
	context.Stdout.Write(t.Bytes())
	poolNames := make([]string, 0, len(poolEntries))
	for poolName := range poolEntries {
		poolNames = append(poolNames, poolName)
	}
	sort.Strings(poolNames)
	for _, poolName := range poolNames {
		entry := poolEntries[poolName]
		t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
		fmt.Fprintf(context.Stdout, "\nEnvironment Variables [%s]:\n", poolName)
		for envName, envValue := range entry.Envs {
			t.AddRow(cmd.Row([]string{envName, fmt.Sprintf("%v", envValue)}))
		}
		t.Sort()
		context.Stdout.Write(t.Bytes())
	}
	return nil
}

type UpgradeCmd struct{}

func (c *UpgradeCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "bs-upgrade",
		Usage: "bs-upgrade",
		Desc: `Upgrades the bs (big sibling) image. You can check the current image with the
[[bs-info]] command.

Running this command will restart the bs container on all nodes and the image
specified at tsuru.conf file will be pulled from the registry.`,
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
