package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
	"sort"
	"strings"
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
		json.Unmarshal([]byte(err.Error()), &result)
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

type removeNodeFromSchedulerCmd struct{}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-node-remove",
		Usage:   "docker-node-remove <address>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
}

func (removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	b, err := json.Marshal(map[string]string{"address": ctx.Args[0]})
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

type listNodesInTheSchedulerCmd struct{}

func (listNodesInTheSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-list",
		Usage: "docker-node-list",
		Desc:  "List available nodes in the cluster",
	}
}

func (listNodesInTheSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
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
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	var nodes []map[string]interface{}
	err = json.Unmarshal(body, &nodes)
	t := cmd.Table{Headers: cmd.Row([]string{"Address", "Status", "Metadata"}), LineSeparator: true}
	for _, n := range nodes {
		addr := n["Address"].(string)
		status := n["Status"].(string)
		result := []string{}
		metadataField, _ := n["Metadata"]
		if metadataField != nil {
			metadata := metadataField.(map[string]interface{})
			for key, value := range metadata {
				result = append(result, fmt.Sprintf("%s=%s", key, value.(string)))
			}
		}
		sort.Strings(result)
		t.AddRow(cmd.Row([]string{addr, status, strings.Join(result, "\n")}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}
