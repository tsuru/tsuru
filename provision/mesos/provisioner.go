// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"io"
	"net/url"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/set"
)

const (
	provisionerName = "mesos"
)

var (
	errNotImplemented = errors.New("not implemented")
	errNotSupported   = errors.New("not supported on mesos")
)

type mesosProvisioner struct{}

func init() {
	provision.Register(provisionerName, func() (provision.Provisioner, error) {
		return &mesosProvisioner{}, nil
	})
}

func (p *mesosProvisioner) GetName() string {
	return provisionerName
}

func (p *mesosProvisioner) Provision(provision.App) error {
	return nil
}

func (p *mesosProvisioner) Destroy(provision.App) error {
	return errNotImplemented
}

func (p *mesosProvisioner) AddUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) RemoveUnits(provision.App, uint, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Restart(provision.App, string, io.Writer) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Start(provision.App, string) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Stop(provision.App, string) error {
	return errNotImplemented
}

func (p *mesosProvisioner) Units(app provision.App) ([]provision.Unit, error) {
	return nil, nil
}

func (p *mesosProvisioner) RoutableAddresses(app provision.App) ([]url.URL, error) {
	return nil, errNotImplemented
}

func (p *mesosProvisioner) RegisterUnit(a provision.App, unitId string, customData map[string]interface{}) error {
	return errNotImplemented
}

func (p *mesosProvisioner) ListNodes(addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	err := forEachCluster(func(c *clusterClient) error {
		clusterNodes, err := p.listNodesForCluster(c, addressFilter)
		if err != nil {
			return err
		}
		nodes = append(nodes, clusterNodes...)
		return nil
	})
	if err == cluster.ErrNoCluster {
		return nil, nil
	}
	if err != nil {
		// TODO(cezarsa): It would be better to return an error to be handled
		// by the api. Failing to list nodes from one provisioner should not
		// prevent other nodes from showing up.
		log.Errorf("unable to list all node from mesos cluster: %v", err)
		return nil, nil
	}
	return nodes, nil
}

func (p *mesosProvisioner) listNodesForCluster(cluster *clusterClient, addressFilter []string) ([]provision.Node, error) {
	var nodes []provision.Node
	var addressSet set.Set
	if len(addressFilter) > 0 {
		addressSet = set.FromSlice(addressFilter)
	}
	state, err := cluster.mesos.GetSlavesFromCluster()
	if err != nil {
		return nil, err
	}
	for i := range state.Slaves {
		n := &mesosNodeWrapper{
			slave:   &state.Slaves[i],
			prov:    p,
			cluster: cluster,
		}
		if addressSet == nil || addressSet.Includes(n.Address()) {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (p *mesosProvisioner) findNodeByAddress(address string) (*clusterClient, *mesosNodeWrapper, error) {
	var (
		foundNode    *mesosNodeWrapper
		foundCluster *clusterClient
	)
	err := forEachCluster(func(c *clusterClient) error {
		if foundNode != nil {
			return nil
		}
		state, err := c.mesos.GetSlavesFromCluster()
		if err != nil {
			return err
		}
		for i := range state.Slaves {
			nodeWrapper := &mesosNodeWrapper{
				slave:   &state.Slaves[i],
				prov:    p,
				cluster: c,
			}
			if address == nodeWrapper.Address() {
				foundNode = nodeWrapper
				foundCluster = c
				break
			}
		}
		return nil
	})
	if err != nil {
		if err == cluster.ErrNoCluster {
			return nil, nil, provision.ErrNodeNotFound
		}
		return nil, nil, err
	}
	if foundNode == nil {
		return nil, nil, provision.ErrNodeNotFound
	}
	return foundCluster, foundNode, nil
}

func (p *mesosProvisioner) GetNode(address string) (provision.Node, error) {
	_, node, err := p.findNodeByAddress(address)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (p *mesosProvisioner) AddNode(opts provision.AddNodeOptions) error {
	return errNotSupported
}

func (p *mesosProvisioner) RemoveNode(opts provision.RemoveNodeOptions) error {
	return errNotSupported
}

func (p *mesosProvisioner) UpdateNode(provision.UpdateNodeOptions) error {
	return errNotSupported
}

func (p *mesosProvisioner) NodeForNodeData(nodeData provision.NodeStatusData) (provision.Node, error) {
	return provision.FindNodeByAddrs(p, nodeData.Addrs)
}

func (p *mesosProvisioner) UploadDeploy(a provision.App, archiveFile io.ReadCloser, fileSize int64, build bool, evt *event.Event) (string, error) {
	return "", errNotImplemented
}
