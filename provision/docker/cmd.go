package docker

import "github.com/tsuru/tsuru/cmd"

type addNodeToSchedulerCmd struct{}

func (addNodeToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-add-node",
		Usage:   "docker-add-node <id> <address> <pool>",
		Desc:    "Registers a new node in the cluster, optionally assigning it to a team",
		MinArgs: 3,
	}
}

func (addNodeToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	cluster := dockerCluster()
	err := cluster.Register(map[string]string{"ID": ctx.Args[0], "address": ctx.Args[1], "pool": ctx.Args[2]})
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully registered.\n"))
	return nil
}

type removeNodeFromSchedulerCmd struct{}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-rm-node",
		Usage:   "docker-rm-node <id>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
}

func (removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	cluster := dockerCluster()
	err := cluster.Unregister(map[string]string{"pool": ctx.Args[0], "address": ctx.Args[1]})
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully removed.\n"))
	return nil
}

type listNodesInTheSchedulerCmd struct{}

func (listNodesInTheSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-list-nodes",
		Usage: "docker-list-nodes",
		Desc:  "List available nodes in the cluster",
	}
}

func (listNodesInTheSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	t := cmd.Table{Headers: cmd.Row([]string{"Address"})}
	nodes, err := listNodesInTheScheduler()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		t.AddRow(cmd.Row([]string{n}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}
