package docker

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/tsuru/cmd"
	"io/ioutil"
	"net/http"
)

type addNodeToSchedulerCmd struct{}

func (addNodeToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-node-add",
		Usage:   "docker-node-add <pool> <address>",
		Desc:    "Registers a new node in the cluster",
		MinArgs: 1,
	}
}

func (addNodeToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	b, err := json.Marshal(map[string]string{"pool": ctx.Args[0], "address": ctx.Args[1]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/node")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully registered.\n"))
	return nil
}

type removeNodeFromSchedulerCmd struct{}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-node-remove",
		Usage:   "docker-node-remove <pool> <address>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
}

func (removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	b, err := json.Marshal(map[string]string{"pool": ctx.Args[0], "address": ctx.Args[1]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/node")
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
	url, err := cmd.GetURL("/node")
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
	var nodes []map[string]string
	err = json.Unmarshal(body, &nodes)
	t := cmd.Table{Headers: cmd.Row([]string{"Address"})}
	for _, n := range nodes {
		t.AddRow(cmd.Row([]string{n["Address"]}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}
