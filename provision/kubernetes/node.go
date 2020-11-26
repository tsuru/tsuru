// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/api/core/v1"
)

type kubernetesNodeWrapper struct {
	node    *apiv1.Node
	prov    *kubernetesProvisioner
	cluster *ClusterClient
	ctx     context.Context
}

var (
	_ provision.Node = &kubernetesNodeWrapper{}
)

func (n *kubernetesNodeWrapper) IaaSID() string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodeIaaSID()
}

func (n *kubernetesNodeWrapper) Pool() string {
	if ok, _ := n.cluster.SinglePool(); len(n.cluster.Pools) > 0 && ok {
		return n.cluster.Pools[0]
	}
	return labelSetFromMeta(&n.node.ObjectMeta).NodePool()
}

func (n *kubernetesNodeWrapper) Address() string {
	if n.node == nil {
		return ""
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
	ctx := n.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	pods, err := appPodsFromNode(ctx, n.cluster, n.node.Name)
	if err != nil {
		return nil, err
	}
	return n.prov.podsToUnits(n.ctx, n.cluster, pods, nil)
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.prov
}

func (n *kubernetesNodeWrapper) RawNode() *apiv1.Node {
	return n.node
}
