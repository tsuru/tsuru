// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/api/core/v1"
)

type kubernetesNodeWrapper struct {
	node    *apiv1.Node
	prov    *kubernetesProvisioner
	cluster *ClusterClient
}

var (
	_ provision.Node = &kubernetesNodeWrapper{}
)

func (n *kubernetesNodeWrapper) IaaSID() string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodeIaaSID()
}

func (n *kubernetesNodeWrapper) Pool() string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodePool()
}

func (n *kubernetesNodeWrapper) Address() string {
	if n.node == nil {
		return ""
	}
	if ip, ok := n.node.Labels["tsuru.io/override-ip"]; ok {
		return ip
	}
	for _, addr := range n.node.Status.Addresses {
		if addr.Type == apiv1.NodeInternalIP {
			return addr.Address
		}
	}
	return n.node.Name
}

func (n *kubernetesNodeWrapper) Status() string {
	for _, t := range n.node.Spec.Taints {
		if t.Key == tsuruNodeDisabledTaint && t.Effect == apiv1.TaintEffectNoSchedule {
			return "Disabled"
		}
	}
	for _, cond := range n.node.Status.Conditions {
		if cond.Type == apiv1.NodeReady {
			if cond.Status == apiv1.ConditionTrue {
				return "Ready"
			}
			return cond.Message
		}
	}
	return "Invalid"
}

func (n *kubernetesNodeWrapper) Metadata() map[string]string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodeMetadata()
}

func (n *kubernetesNodeWrapper) MetadataNoPrefix() map[string]string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodeMetadataNoPrefix()
}

func (n *kubernetesNodeWrapper) ExtraData() map[string]string {
	var clusterName string
	if n.cluster != nil {
		clusterName = n.cluster.Name
	}
	return labelSetFromMeta(&n.node.ObjectMeta).NodeExtraData(clusterName)
}

func (n *kubernetesNodeWrapper) Units() ([]provision.Unit, error) {
	pods, err := appPodsFromNode(n.cluster, n.node.Name)
	if err != nil {
		return nil, err
	}
	return n.prov.podsToUnits(n.cluster, pods, nil, n.node)
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}

func (n *kubernetesNodeWrapper) ip() string {
	if n.node == nil {
		return ""
	}
	return tsuruNet.URLToHost(n.Address())
}
