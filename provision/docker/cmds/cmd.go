// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmds

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/provision/docker/types"
)

func init() {
	cmd.RegisterExtraCmd(&moveContainerCmd{})
	cmd.RegisterExtraCmd(&moveContainersCmd{})
	cmd.RegisterExtraCmd(&dockerLogInfo{})
	cmd.RegisterExtraCmd(&dockerLogUpdate{})
}

type dockerLogUpdate struct {
	cmd.ConfirmationCommand
	fs        *gnuflag.FlagSet
	pool      string
	restart   bool
	logDriver string
	logOpts   cmd.MapFlag
}

func (c *dockerLogUpdate) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		desc := "Pool name where log options will be used."
		c.fs.StringVar(&c.pool, "pool", "", desc)
		c.fs.StringVar(&c.pool, "p", "", desc)
		desc = "Whether tsuru should restart all apps on the specified pool."
		c.fs.BoolVar(&c.restart, "restart", false, desc)
		c.fs.BoolVar(&c.restart, "r", false, desc)
		desc = "Log options send to the specified log-driver"
		c.fs.Var(&c.logOpts, "log-opt", desc)
		desc = "Chosen log driver. Supported log drivers depend on the docker version running on nodes."
		c.fs.StringVar(&c.logDriver, "log-driver", "", desc)
	}
	return c.fs
}

func (c *dockerLogUpdate) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-log-update",
		Usage: "docker-log-update [-r/--restart] [-p/--pool poolname] --log-driver <driver> [--log-opt name=value]...",
		Desc: `Set custom configuration for container logs. By default tsuru configures
application containers to send all logs to the tsuru/bs container through
syslog.

Setting a custom log-driver allow users to change this behavior and make
containers send their logs directly using the driver bypassing tsuru/bs
completely. In this situation the 'tsuru app-log' command will not work
anymore.

The --log-driver option accepts either the value 'bs' restoring tsuru default
behavior or any log-driver supported by docker along with their --log-opt. See
https://docs.docker.com/engine/reference/logging/overview/ for more details.

If --pool is specified the log-driver will only be used on containers started
on the chosen pool.`,
		MinArgs: 0,
	}
}

func (c *dockerLogUpdate) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	if c.restart {
		extra := ""
		if c.pool != "" {
			extra = fmt.Sprintf(" running on pool %s", c.pool)
		}
		msg := fmt.Sprintf("Are you sure you want to restart all apps%s?", extra)
		if !c.Confirm(context, msg) {
			return nil
		}
	}
	u, err := cmd.GetURL("/docker/logs")
	if err != nil {
		return err
	}
	conf := types.DockerLogConfig{
		Driver:  c.logDriver,
		LogOpts: map[string]string(c.logOpts),
	}
	values, err := form.EncodeToValues(conf)
	if err != nil {
		return err
	}
	values.Set("pool", c.pool)
	values.Set("restart", strconv.FormatBool(c.restart))
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", u, reader)
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

type dockerLogInfo struct{}

func (c *dockerLogInfo) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-log-info",
		Usage:   "docker-log-info",
		Desc:    "Prints information about docker log configuration for each pool.",
		MinArgs: 0,
	}
}

func (c *dockerLogInfo) Run(context *cmd.Context, client *cmd.Client) error {
	u, err := cmd.GetURL("/docker/logs")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var conf map[string]types.DockerLogConfig
	err = json.NewDecoder(response.Body).Decode(&conf)
	if err != nil {
		return err
	}
	baseConf := conf[""]
	delete(conf, "")
	t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
	fmt.Fprintf(context.Stdout, "Log driver [default]: %s\n", baseConf.Driver)
	for optName, optValue := range baseConf.LogOpts {
		t.AddRow(cmd.Row([]string{optName, optValue}))
	}
	if t.Rows() > 0 {
		t.Sort()
		context.Stdout.Write(t.Bytes())
	}
	poolNames := make([]string, 0, len(baseConf.LogOpts))
	for poolName := range conf {
		poolNames = append(poolNames, poolName)
	}
	sort.Strings(poolNames)
	for _, poolName := range poolNames {
		poolConf := conf[poolName]
		t := cmd.Table{Headers: cmd.Row([]string{"Name", "Value"})}
		fmt.Fprintf(context.Stdout, "\nLog driver [pool %s]: %s\n", poolName, poolConf.Driver)
		for optName, optValue := range poolConf.LogOpts {
			t.AddRow(cmd.Row([]string{optName, optValue}))
		}
		if t.Rows() > 0 {
			t.Sort()
			context.Stdout.Write(t.Bytes())
		}
	}
	return nil
}
