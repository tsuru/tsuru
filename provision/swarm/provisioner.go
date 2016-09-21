// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"errors"
	"fmt"
	"io"

	"github.com/docker/engine-api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
)

const (
	provisionerName = "swarm"

	labelInternalPrefix = "tsuru-internal-"
	labelDockerAddr     = labelInternalPrefix + "docker-addr"
)

var errNotImplemented = errors.New("not implemented")

type swarmProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &swarmProvisioner{}, nil
	})
}

func (p *swarmProvisioner) GetName() string {
	return provisionerName
}

func (p *swarmProvisioner) Provision(provision.App) error {
	return nil
}

func (p *swarmProvisioner) Destroy(provision.App) error {
	return errNotImplemented
}

func (p *swarmProvisioner) AddUnits(provision.App, uint, string, io.Writer) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *swarmProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *swarmProvisioner) SetUnitStatus(provision.Unit, provision.Status) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Restart(provision.App, string, io.Writer) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Start(provision.App, string) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Stop(provision.App, string) error {
	return errNotImplemented
}

func (p *swarmProvisioner) Units(provision.App) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *swarmProvisioner) RoutableUnits(provision.App) ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (p *swarmProvisioner) RegisterUnit(provision.Unit, map[string]interface{}) error {
	return errNotImplemented
}

func (p *swarmProvisioner) SetNodeStatus(provision.NodeStatusData) error {
	return errNotImplemented
}

func (p *swarmProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	client, _, err := chooseDBSwarmNode()
	if err != nil {
		if err == errNoSwarmNode {
			return nil, nil
		}
		return nil, err
	}
	nodes, err := client.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return nil, err
	}
	var filterMap map[string]struct{}
	if len(addressFilter) > 0 {
		filterMap = map[string]struct{}{}
		for _, addr := range addressFilter {
			filterMap[tsuruNet.URLToHost(addr)] = struct{}{}
		}
	}
	nodeList := make([]provision.Node, 0, len(nodes))
	for i := range nodes {
		wrapped := &swarmNodeWrapper{Node: &nodes[i]}
		toAdd := true
		if filterMap != nil {
			_, toAdd = filterMap[tsuruNet.URLToHost(wrapped.Address())]
		}
		if toAdd {
			nodeList = append(nodeList, wrapped)
		}
	}
	return nodeList, nil
}

func (p *swarmProvisioner) GetNode(address string) (provision.Node, error) {
	nodes, err := p.ListNodes([]string{address})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, provision.ErrNodeNotFound
	}
	return nodes[0], nil
}

func (p *swarmProvisioner) AddNode(opts provision.AddNodeOptions) error {
	existingClient, existingAddr, err := chooseDBSwarmNode()
	if err != nil && err != errNoSwarmNode {
		return err
	}
	newClient, err := newClient(opts.Address)
	if err != nil {
		return err
	}
	host := tsuruNet.URLToHost(opts.Address)
	if existingClient == nil {
		_, err = newClient.InitSwarm(docker.InitSwarmOptions{
			InitRequest: swarm.InitRequest{
				ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
				AdvertiseAddr: host,
			},
		})
		if err == docker.ErrNodeAlreadyInSwarm {
			err = nil
		}
	} else {
		var swarmInfo swarm.Swarm
		swarmInfo, err = existingClient.InspectSwarm(nil)
		if err != nil {
			return err
		}
		opts := docker.JoinSwarmOptions{
			JoinRequest: swarm.JoinRequest{
				ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
				AdvertiseAddr: host,
				JoinToken:     swarmInfo.JoinTokens.Manager,
				RemoteAddrs:   []string{existingAddr.SwarmAddress},
			},
		}
		err = newClient.JoinSwarm(opts)
	}
	if err != nil {
		return err
	}
	dockerInfo, err := newClient.Info()
	if err != nil {
		return err
	}
	nodeData, err := newClient.InspectNode(dockerInfo.Swarm.NodeID)
	if err != nil {
		return err
	}
	nodeData.Spec.Annotations.Labels = map[string]string{
		labelDockerAddr: opts.Address,
	}
	for k, v := range opts.Metadata {
		nodeData.Spec.Annotations.Labels[k] = v
	}
	err = newClient.UpdateNode(dockerInfo.Swarm.NodeID, docker.UpdateNodeOptions{
		Version:  nodeData.Version.Index,
		NodeSpec: nodeData.Spec,
	})
	if err != nil {
		return err
	}
	return updateDBSwarmNodes(newClient)
}

func (p *swarmProvisioner) RemoveNode(provision.RemoveNodeOptions) error {
	return errNotImplemented
}

func (p *swarmProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return errNotImplemented
}
