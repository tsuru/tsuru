package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tsuru/tsuru/cmd"
	"launchpad.net/gnuflag"
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

type removeNodeFromSchedulerCmd struct {
	cmd.ConfirmationCommand
	fs      *gnuflag.FlagSet
	destroy bool
}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-remove",
		Usage: "docker-node-remove <address> [--destroy] [-y]",
		Desc: `Removes a node from the cluster.

--destroy: Destroy the machine in the IaaS used to create it, if it exists.
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
	url, err := cmd.GetURL("/docker/node")
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
		m, ok := machineMap[urlToHost(addr)]
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

type listHealingHistoryCmd struct {
	fs            *gnuflag.FlagSet
	nodeOnly      bool
	containerOnly bool
}

func (c *listHealingHistoryCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
}

func renderHistoryTable(history []healingEvent, filter string, ctx *cmd.Context) {
	fmt.Fprintln(ctx.Stdout, strings.ToUpper(filter[:1])+filter[1:]+":")
	headers := cmd.Row([]string{"Start", "Finish", "Success", "Failing", "Created", "Error"})
	t := cmd.Table{Headers: headers}
	for i := len(history) - 1; i >= 0; i-- {
		event := history[i]
		if event.Action != filter+"-healing" {
			continue
		}
		data := make([]string, 2)
		if filter == "node" {
			data[0] = event.FailingNode.Address
			data[1] = event.CreatedNode.Address
		} else {
			data[0] = event.FailingContainer.ID
			data[1] = event.CreatedContainer.ID
			if len(data[0]) > 10 {
				data[0] = data[0][:10]
			}
			if len(data[1]) > 10 {
				data[1] = data[1][:10]
			}
		}
		t.AddRow(cmd.Row([]string{
			event.StartTime.Local().Format(time.Stamp),
			event.EndTime.Local().Format(time.Stamp),
			fmt.Sprintf("%t", event.Successful),
			data[0],
			data[1],
			event.Error,
		}))
	}
	t.LineSeparator = true
	ctx.Stdout.Write(t.Bytes())
}

func (c *listHealingHistoryCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var filter string
	if c.nodeOnly && !c.containerOnly {
		filter = "node"
	}
	if c.containerOnly && !c.nodeOnly {
		filter = "container"
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/healing?filter=%s", filter))
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
	var history []healingEvent
	err = json.NewDecoder(resp.Body).Decode(&history)
	if err != nil {
		return err
	}
	if filter != "" {
		renderHistoryTable(history, filter, ctx)
	} else {
		renderHistoryTable(history, "node", ctx)
		renderHistoryTable(history, "container", ctx)
	}
	return nil
}

func (c *listHealingHistoryCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.BoolVar(&c.nodeOnly, "node", false, "List only healing process started for nodes")
		c.fs.BoolVar(&c.containerOnly, "container", false, "List only healing process started for containers")
	}
	return c.fs
}
