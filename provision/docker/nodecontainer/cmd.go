// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"github.com/tsuru/tsuru/cmd"
)

type NodeContainerList struct{}

func (c *NodeContainerList) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-list",
	}
}

func (c *NodeContainerList) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}

type NodeContainerAdd struct{}

func (c *NodeContainerAdd) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-add",
	}
}

func (c *NodeContainerAdd) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}

type NodeContainerInfo struct{}

func (c *NodeContainerInfo) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-info",
	}
}

func (c *NodeContainerInfo) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}

type NodeContainerUpdate struct{}

func (c *NodeContainerUpdate) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-update",
	}
}

func (c *NodeContainerUpdate) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}

type NodeContainerDelete struct{}

func (c *NodeContainerDelete) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-delete",
	}
}

func (c *NodeContainerDelete) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}

type NodeContainerUpgrade struct{}

func (c *NodeContainerUpgrade) Info() *cmd.Info {
	//TODO(cezarsa) fill the blank
	return &cmd.Info{
		Name: "node-container-upgrade",
	}
}

func (c *NodeContainerUpgrade) Run(context *cmd.Context, client *cmd.Client) error {
	//TODO(cezarsa) fill the blank
	return nil
}
