// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/provision"
	"k8s.io/client-go/pkg/api/v1"
)

type kubernetesNodeWrapper struct {
	node    *v1.Node
	prov    *kubernetesProvisioner
	cluster *Cluster
}

var (
	_ provision.Node = &kubernetesNodeWrapper{}
)

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
	pods, err := podsFromNode(n.cluster, n.node.Name)
	if err != nil {
		return nil, err
	}
	return n.prov.podsToUnits(n.cluster, pods, nil, n.node)
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}
