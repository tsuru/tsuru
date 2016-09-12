// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/provision/docker/container"
)

type listAutoScaleHistoryCmd struct {
	fs   *gnuflag.FlagSet
	page int
}

func (c *listAutoScaleHistoryCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-list",
		Usage: "docker-autoscale-list [--page/-p 1]",
		Desc:  "List node auto scale history.",
	}
}

func (c *listAutoScaleHistoryCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	if c.page < 1 {
		c.page = 1
	}
	limit := 20
	skip := (c.page - 1) * limit
	u, err := cmd.GetURL(fmt.Sprintf("/docker/autoscale?skip=%d&limit=%d", skip, limit))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var history []autoScaleEvent
	if resp.StatusCode == 204 {
		ctx.Stdout.Write([]byte("There is no auto scales yet.\n"))
		return nil
	}
	err = json.NewDecoder(resp.Body).Decode(&history)
	if err != nil {
		return err
	}
	headers := cmd.Row([]string{"Start", "Finish", "Success", "Metadata", "Action", "Reason", "Error"})
	t := cmd.Table{Headers: headers}
	for i := range history {
		event := &history[i]
		t.AddRow(cmd.Row([]string{
			event.StartTime.Local().Format(time.Stamp),
			event.EndTime.Local().Format(time.Stamp),
			fmt.Sprintf("%t", event.Successful),
			event.MetadataValue,
			event.Action,
			event.Reason,
			event.Error,
		}))
	}
	t.LineSeparator = true
	ctx.Stdout.Write(t.Bytes())
	return nil
}

func (c *listAutoScaleHistoryCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.IntVar(&c.page, "page", 1, "Current page")
		c.fs.IntVar(&c.page, "p", 1, "Current page")
	}
	return c.fs
}

type autoScaleRunCmd struct {
	cmd.ConfirmationCommand
}

func (c *autoScaleRunCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-run",
		Usage: "docker-autoscale-run [-y/--assume-yes]",
		Desc: `Run node auto scale checks once. This command will work even if [[docker:auto-
scale:enabled]] config entry is set to false. Auto scaling checks may trigger
the addition, removal or rebalancing of docker nodes, as long as these nodes
were created using an IaaS provider registered in tsuru.`,
	}
}

func (c *autoScaleRunCmd) Run(context *cmd.Context, client *cmd.Client) error {
	context.RawOutput()
	if !c.Confirm(context, "Are you sure you want to run auto scaling checks?") {
		return nil
	}
	u, err := cmd.GetURL("/docker/autoscale/run")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return err
	}
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

type autoScaleInfoCmd struct{}

func (c *autoScaleInfoCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-info",
		Usage: "docker-autoscale-info",
		Desc: `Display the current configuration for tsuru autoscale,
including the set of rules and the current metadata filter.

The metadata filter is the value that defines which node metadata will be used
to group autoscale rules. A common approach is to use the "pool" as the
filter. Then autoscale can be configured for each matching rule value.`,
	}
}

func (c *autoScaleInfoCmd) Run(context *cmd.Context, client *cmd.Client) error {
	config, err := c.getAutoScaleConfig(client)
	if err != nil {
		return err
	}
	if !config.Enabled {
		fmt.Fprintln(context.Stdout, "auto-scale is disabled")
		return nil
	}
	rules, err := c.getAutoScaleRules(client)
	if err != nil {
		return err
	}
	return c.render(context, config, rules)
}

func (c *autoScaleInfoCmd) getAutoScaleConfig(client *cmd.Client) (*autoScaleConfig, error) {
	u, err := cmd.GetURL("/docker/autoscale/config")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var config autoScaleConfig
	err = json.NewDecoder(resp.Body).Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *autoScaleInfoCmd) getAutoScaleRules(client *cmd.Client) ([]autoScaleRule, error) {
	u, err := cmd.GetURL("/docker/autoscale/rules")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rules []autoScaleRule
	err = json.NewDecoder(resp.Body).Decode(&rules)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *autoScaleInfoCmd) render(context *cmd.Context, config *autoScaleConfig, rules []autoScaleRule) error {
	var table cmd.Table
	tableHeader := []string{
		"Pool",
		"Max container count",
		"Max memory ratio",
		"Scale down ratio",
		"Rebalance on scale",
		"Enabled",
	}
	table.Headers = tableHeader
	for _, rule := range rules {
		table.AddRow([]string{
			rule.MetadataFilter,
			strconv.Itoa(rule.MaxContainerCount),
			strconv.FormatFloat(float64(rule.MaxMemoryRatio), 'f', 4, 32),
			strconv.FormatFloat(float64(rule.ScaleDownRatio), 'f', 4, 32),
			strconv.FormatBool(!rule.PreventRebalance),
			strconv.FormatBool(rule.Enabled),
		})
	}
	fmt.Fprintf(context.Stdout, "Rules:\n%s", table.String())
	return nil
}

type autoScaleSetRuleCmd struct {
	fs                 *gnuflag.FlagSet
	filterValue        string
	maxContainerCount  int
	maxMemoryRatio     float64
	scaleDownRatio     float64
	noRebalanceOnScale bool
	enable             bool
	disable            bool
}

func (c *autoScaleSetRuleCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-rule-set",
		Usage: "docker-autoscale-rule-set [-f/--filter-value <pool name>] [-c/--max-container-count 0] [-m/--max-memory-ratio 0.9] [-d/--scale-down-ratio 1.33] [--no-rebalance-on-scale] [--enable] [--disable]",
		Desc:  "Creates or update an auto-scale rule. Using resources limitation (amount of container or memory usage).",
	}
}

func (c *autoScaleSetRuleCmd) Run(context *cmd.Context, client *cmd.Client) error {
	if (c.enable && c.disable) || (!c.enable && !c.disable) {
		return errors.New("either --disable or --enable must be set")
	}
	rule := autoScaleRule{
		MetadataFilter:    c.filterValue,
		MaxContainerCount: c.maxContainerCount,
		MaxMemoryRatio:    float32(c.maxMemoryRatio),
		ScaleDownRatio:    float32(c.scaleDownRatio),
		PreventRebalance:  c.noRebalanceOnScale,
		Enabled:           c.enable,
	}
	val, err := form.EncodeToValues(rule)
	if err != nil {
		return err
	}
	body := strings.NewReader(val.Encode())
	u, err := cmd.GetURL("/docker/autoscale/rules")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Rule successfully defined.")
	return nil
}

func (c *autoScaleSetRuleCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("autoscale-rule-set", gnuflag.ExitOnError)
		msg := "The pool name matching the rule. This is the unique identifier of the rule."
		c.fs.StringVar(&c.filterValue, "filter-value", "", msg)
		c.fs.StringVar(&c.filterValue, "f", "", msg)
		msg = "The maximum amount of containers on every node. Might be zero, which means no maximum value. Whenever this value is reached, tsuru will trigger a new auto scale event."
		c.fs.IntVar(&c.maxContainerCount, "max-container-count", 0, msg)
		c.fs.IntVar(&c.maxContainerCount, "c", 0, msg)
		msg = "The maximum memory usage per node. 0 means no limit, 1 means 100%. It is fine to use values greater than 1, which means that tsuru will overcommit memory in Docker nodes. Keep in mind that container count has higher precedence than memory ratio, so if --max-container-count is defined, the value of --max-memory-ratio will be ignored."
		c.fs.Float64Var(&c.maxMemoryRatio, "max-memory-ratio", .0, msg)
		c.fs.Float64Var(&c.maxMemoryRatio, "m", .0, msg)
		msg = "The ratio for triggering an scale down event. The default value is 1.33, which mean that whenever it gets one third of the resource utilization (memory ratio or container count)."
		c.fs.Float64Var(&c.scaleDownRatio, "scale-down-ratio", 1.33, msg)
		c.fs.Float64Var(&c.scaleDownRatio, "d", 1.33, msg)
		msg = "A boolean flag indicating whether containers should NOT be rebalanced after running an scale. The default behavior is to always rebalance the containers."
		c.fs.BoolVar(&c.noRebalanceOnScale, "no-rebalance-on-scale", false, msg)
		msg = "A boolean flag indicating whether the rule should be enabled"
		c.fs.BoolVar(&c.enable, "enable", false, msg)
		msg = "A boolean flag indicating whether the rule should be disabled"
		c.fs.BoolVar(&c.disable, "disable", false, msg)
	}
	return c.fs
}

type autoScaleDeleteRuleCmd struct {
	cmd.ConfirmationCommand
}

func (c *autoScaleDeleteRuleCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-rule-remove",
		Usage: "docker-autoscale-rule-remove [rule-name] [-y/--assume-yes]",
		Desc:  `Removes an auto-scale rule. The name of the rule may be omitted, which means "remove the default rule".`,
	}
}

func (c *autoScaleDeleteRuleCmd) Run(context *cmd.Context, client *cmd.Client) error {
	var rule string
	confirmMsg := "Are you sure you want to remove the default rule?"
	if len(context.Args) > 0 {
		rule = context.Args[0]
		confirmMsg = fmt.Sprintf("Are you sure you want to remove the rule %q?", rule)
	}
	if !c.Confirm(context, confirmMsg) {
		return nil
	}
	u, err := cmd.GetURL("/docker/autoscale/rules/" + rule)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Rule successfully removed.")
	return nil
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
	conf := container.DockerLogConfig{
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
	var conf map[string]container.DockerLogConfig
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
