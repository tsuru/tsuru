package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
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

type removeNodeFromSchedulerCmd struct {
	fs      *gnuflag.FlagSet
	destroy bool
}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-node-remove",
		Usage: "docker-node-remove <address> [--destroy]",
		Desc: `Removes a node from the cluster.

--destroy: Destroy the machine in the IaaS used to create it, if it exists.
`,
		MinArgs: 1,
	}
}

func (c *removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
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
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.BoolVar(&c.destroy, "destroy", false, "Destroy node from IaaS")
	}
	return c.fs
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
		if metadataField != nil {
			metadata := metadataField.(map[string]interface{})
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
