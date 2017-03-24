// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/provision"
	"k8s.io/client-go/pkg/api/v1"
)

type kubernetesNodeWrapper struct {
	node *v1.Node
	prov *kubernetesProvisioner
}

func (n *kubernetesNodeWrapper) Pool() string {
	if n.node.Labels == nil {
		return ""
	}
	l := provision.LabelSet{Labels: n.node.Labels}
	return l.NodePool()
}

func (n *kubernetesNodeWrapper) Address() string {
	for _, addr := range n.node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func (n *kubernetesNodeWrapper) Status() string {
	for _, cond := range n.node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			if cond.Status == v1.ConditionTrue {
				return "Ready"
			}
			return cond.Message
		}
	}
	return "Invalid"
}

func (n *kubernetesNodeWrapper) Metadata() map[string]string {
	if n.node.Labels == nil {
		return map[string]string{}
	}
	return n.node.Labels
}

func (n *kubernetesNodeWrapper) Units() ([]provision.Unit, error) {
	client, err := getClusterClient()
	if err != nil {
		return nil, err
	}
	pods, err := podsFromNode(client, n.node.Name)
	if err != nil {
		return nil, err
	}
	return n.prov.podsToUnits(client, pods, nil, n.node)
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}

type clusterNode struct {
	address string
	prov    *kubernetesProvisioner
}

func (n *clusterNode) Pool() string {
	return ""
}

func (n *clusterNode) Address() string {
	return n.address
}

func (n *clusterNode) Status() string {
	return "Ready"
}

func (n *clusterNode) Metadata() map[string]string {
	return map[string]string{"cluster": "true"}
}

func (n *clusterNode) Units() ([]provision.Unit, error) {
	return nil, nil
}

func (n *clusterNode) Provisioner() provision.NodeProvisioner {
	return n.prov
}
