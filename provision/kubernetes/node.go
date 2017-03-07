// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/provision"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	labelNodePoolName = "pool"
)

type kubernetesNodeWrapper struct {
	node *v1.Node
	prov *kubernetesProvisioner
}

func (n *kubernetesNodeWrapper) Pool() string {
	if n.node.Labels == nil {
		return ""
	}
	return n.node.Labels[labelNodePoolName]
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
	return nil, errNotImplemented
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}
