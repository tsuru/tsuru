// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/net"
)

type addNodeToSchedulerCmd struct {
	fs       *gnuflag.FlagSet
	register bool
}

func (addNodeToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-add",
		Usage: "docker-node-add [param_name=param_value]... [--register]",
		Desc: `Creates or registers a new node in the cluster.
By default, this command will call the configured IaaS to create a new
machine. Every param will be sent to the IaaS implementation.

Parameters with special meaning:
  iaas=<iaas name>          Which iaas provider should be used, if not set
                            tsuru will use the default iaas specified in
                            tsuru.conf file.
  template=<template name>  A machine template with predefined parameters,
                            additional parameters will override template
                            ones. See 'machine-template-add' command.

--register: Registers an existing docker endpoint. The IaaS won't be called.
            Having a address=<docker_api_url> param is mandatory.
`,
		MinArgs: 0,
	}
}

func (a *addNodeToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	jsonParams := map[string]string{}
	for _, param := range ctx.Args {
		if strings.Contains(param, "=") {
			keyValue := strings.SplitN(param, "=", 2)
			jsonParams[keyValue[0]] = keyValue[1]
		}
	}
	b, err := json.Marshal(jsonParams)
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/node?register=%t", a.register))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		result := make(map[string]string)
		unmarshalErr := json.Unmarshal([]byte(err.Error()), &result)
		if unmarshalErr != nil {
			return err
		}
		fmt.Fprintf(ctx.Stderr, "Error: %s\n\n%s\n", result["error"], result["description"])
		return nil
	}
	ctx.Stdout.Write([]byte("Node successfully registered.\n"))
	return nil
}

func (a *addNodeToSchedulerCmd) Flags() *gnuflag.FlagSet {
	if a.fs == nil {
		a.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		a.fs.BoolVar(&a.register, "register", false, "Register an already created node")
	}
	return a.fs
}

type updateNodeToSchedulerCmd struct {
	fs       *gnuflag.FlagSet
	disabled bool
	enabled  bool
}

func (updateNodeToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-update",
		Usage: "docker-node-update <address> [param_name=param_value...] --disable",
		Desc: `Modifies metadata associated to a docker node.
--disable: Disable node in scheduler.
--enable: Enable node in scheduler.`,
		MinArgs: 1,
	}
}

func (a *updateNodeToSchedulerCmd) Flags() *gnuflag.FlagSet {
	if a.fs == nil {
		a.fs = gnuflag.NewFlagSet("", gnuflag.ExitOnError)
		a.fs.BoolVar(&a.disabled, "disable", false, "Disable node in scheduler.")
		a.fs.BoolVar(&a.enabled, "enable", false, "Enable node in scheduler.")
	}
	return a.fs
}

func (a *updateNodeToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	jsonParams := map[string]string{}
	for _, param := range ctx.Args[1:] {
		if strings.Contains(param, "=") {
			keyValue := strings.SplitN(param, "=", 2)
			jsonParams[keyValue[0]] = keyValue[1]
		}
	}
	jsonParams["address"] = ctx.Args[0]
	b, err := json.Marshal(jsonParams)
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/node?disabled=%t&enabled=%t", a.disabled, a.enabled))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully updated.\n"))
	return nil
}

type removeNodeFromSchedulerCmd struct {
	cmd.ConfirmationCommand
	fs          *gnuflag.FlagSet
	destroy     bool
	noRebalance bool
}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-remove",
		Usage: "docker-node-remove <address> [--no-rebalance] [--destroy] [-y]",
		Desc: `Removes a node from the cluster.

--destroy: Destroy the machine in the IaaS used to create it, if it exists.
--no-rebalance: Do not rebalance containers of removed node.
`,
		MinArgs: 1,
	}
}

func (c *removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	msg := "Are you sure you sure you want to remove \"%s\" from cluster"
	if c.destroy {
		msg += " and DESTROY the machine from IaaS"
	}
	if !c.Confirm(ctx, fmt.Sprintf(msg+"?", ctx.Args[0])) {
		return nil
	}
	params := map[string]string{"address": ctx.Args[0]}
	if c.destroy {
		params["remove_iaas"] = "true"
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/node?no-rebalance=%t", c.noRebalance))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully removed.\n"))
	return nil
}

func (c *removeNodeFromSchedulerCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.ConfirmationCommand.Flags()
		c.fs.BoolVar(&c.destroy, "destroy", false, "Destroy node from IaaS")
		c.fs.BoolVar(&c.noRebalance, "no-rebalance", false, "Do not rebalance containers of removed node.")
	}
	return c.fs
}

type listNodesInTheSchedulerCmd struct {
	fs     *gnuflag.FlagSet
	filter cmd.MapFlag
}

func (c *listNodesInTheSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-list",
		Usage: "docker-node-list [--filter/-f <metadata>=<value>]...",
		Desc:  "List available nodes in the cluster",
	}
}

func (c *listNodesInTheSchedulerCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		filter := "Filter by metadata value"
		c.fs.Var(&c.filter, "filter", filter)
		c.fs.Var(&c.filter, "f", filter)
	}
	return c.fs
}

func (c *listNodesInTheSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/node")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return err
	}
	machineMap := map[string]map[string]interface{}{}
	if result["machines"] != nil {
		machines := result["machines"].([]interface{})
		for _, m := range machines {
			machine := m.(map[string]interface{})
			machineMap[machine["Address"].(string)] = m.(map[string]interface{})
		}
	}
	t := cmd.Table{Headers: cmd.Row([]string{"Address", "IaaS ID", "Status", "Metadata"}), LineSeparator: true}
	var nodes []interface{}
	if result["nodes"] != nil {
		nodes = result["nodes"].([]interface{})
	}
	for _, n := range nodes {
		node := n.(map[string]interface{})
		addr := node["Address"].(string)
		status := node["Status"].(string)
		result := []string{}
		metadataField, _ := node["Metadata"]
		if c.filter != nil && metadataField == nil {
			continue
		}
		if metadataField != nil {
			metadata := metadataField.(map[string]interface{})
			valid := true
			for key, value := range c.filter {
				metaVal, _ := metadata[key]
				if metaVal != value {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			for key, value := range metadata {
				result = append(result, fmt.Sprintf("%s=%s", key, value.(string)))
			}
		}
		sort.Strings(result)
		m, ok := machineMap[net.URLToHost(addr)]
		var iaasId string
		if ok {
			iaasId = m["Id"].(string)
		}
		t.AddRow(cmd.Row([]string{addr, iaasId, status, strings.Join(result, "\n")}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}

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
	url, err := cmd.GetURL(fmt.Sprintf("/docker/autoscale?skip=%d&limit=%d", skip, limit))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var history []autoScaleEvent
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
	url, err := cmd.GetURL("/docker/autoscale/run")
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
	url, err := cmd.GetURL("/docker/autoscale/config")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest("GET", url, nil)
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
	url, err := cmd.GetURL("/docker/autoscale/rules")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest("GET", url, nil)
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
	fmt.Fprintf(context.Stdout, "Metadata filter: %s\n\n", config.GroupByMetadata)
	var table cmd.Table
	tableHeader := []string{
		"Filter value",
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
	fs                *gnuflag.FlagSet
	filterValue       string
	maxContainerCount int
	maxMemoryRatio    float64
	scaleDownRatio    float64
	rebalanceOnScale  bool
	enabled           bool
}

func (c *autoScaleSetRuleCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-autoscale-rule-set",
		Usage: "docker-autoscale-rule-set [-f/--filter-value metadata-filter-value] [-c/--max-container-count 0] [-m/--max-memory-ratio 0.9] [-d/--scale-down-ratio 1.33] [-r/--rebalance-on-scale false] [-e/--enabled true]",
		Desc:  "Creates or update an auto-scale rule. Using resources limitation (amount of container or memory usage).",
	}
}

func (c *autoScaleSetRuleCmd) Run(context *cmd.Context, client *cmd.Client) error {
	rule := autoScaleRule{
		MetadataFilter:    c.filterValue,
		MaxContainerCount: c.maxContainerCount,
		MaxMemoryRatio:    float32(c.maxMemoryRatio),
		ScaleDownRatio:    float32(c.scaleDownRatio),
		PreventRebalance:  !c.rebalanceOnScale,
		Enabled:           c.enabled,
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	body := bytes.NewBuffer(data)
	url, err := cmd.GetURL("/docker/autoscale/rules")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
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
		c.fs.StringVar(&c.filterValue, "filter-value", "", "The value of the metadata filter matching the rule. This is the unique identifier of the rule.")
		c.fs.StringVar(&c.filterValue, "f", "", "The value of the metadata filter matching the rule. This is the unique identifier of the rule.")
		c.fs.IntVar(&c.maxContainerCount, "max-container-count", 0, "The maximum amount of containers on every node. Might be zero, which means no maximum value. Whenever this value is reached, tsuru will trigger a new auto scale event.")
		c.fs.IntVar(&c.maxContainerCount, "c", 0, "The maximum amount of containers on every node. Might be zero, which means no maximum value. Whenever this value is reached, tsuru will trigger a new auto scale event.")
		c.fs.Float64Var(&c.maxMemoryRatio, "max-memory-ratio", .0, "The maximum memory usage per node. 0 means no limit, 1 means 100%. It is fine to use values greater than 1, which means that tsuru will overcommit memory in Docker nodes. Keep in mind that container count has higher precedence than memory ratio, so if --max-container-count is defined, the value of --max-memory-ratio will be ignored.")
		c.fs.Float64Var(&c.maxMemoryRatio, "m", .0, "The maximum memory usage per node. 0 means no limit, 1 means 100%. It is fine to use values greater than 1, which means that tsuru will overcommit memory in Docker nodes. Keep in mind that container count has higher precedence than memory ratio, so if --max-container-count is defined, the value of --max-memory-ratio will be ignored.")
		c.fs.Float64Var(&c.scaleDownRatio, "scale-down-ratio", 1.33, "The ratio for triggering an scale down event. The default value is 1.33, which mean that whenever it gets one third of the resource utilization (memory ratio or container count).")
		c.fs.Float64Var(&c.scaleDownRatio, "d", 1.33, "The ratio for triggering an scale down event. The default value is 1.33, which mean that whenever it gets one third of the resource utilization (memory ratio or container count).")
		c.fs.BoolVar(&c.rebalanceOnScale, "rebalance-on-scale", true, "A boolean flag indicating whether containers should be rebalanced after running an scale. The default behavior is to always rebalance the containers.")
		c.fs.BoolVar(&c.rebalanceOnScale, "r", true, "A boolean flag indicating whether containers should be rebalanced after running an scale. The default behavior is to always rebalance the containers.")
		c.fs.BoolVar(&c.enabled, "enabled", true, "A boolean flag indicating whether the rule should be enabled or disabled")
		c.fs.BoolVar(&c.enabled, "e", true, "A boolean flag indicating whether the rule should be enabled or disabled")
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
		Desc:  `Removes an auto-scale rule. The name of the rule may be omited, which means "remove the default rule".`,
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
	url, err := cmd.GetURL("/docker/autoscale/rules/" + rule)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, nil)
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
