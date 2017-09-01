// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"strings"

	"github.com/tsuru/tsuru/provision"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

type kubernetesNodeWrapper struct {
	node    *apiv1.Node
	prov    *kubernetesProvisioner
	cluster *clusterClient
}

var (
	_ provision.Node = &kubernetesNodeWrapper{}
)

func (n *kubernetesNodeWrapper) Pool() string {
	return labelSetFromMeta(&n.node.ObjectMeta).NodePool()
}

func (n *kubernetesNodeWrapper) Address() string {
	for _, addr := range n.node.Status.Addresses {
		if addr.Type == apiv1.NodeInternalIP {
			return addr.Address
		}
	}
	return n.node.Name
}

func (n *kubernetesNodeWrapper) Status() string {
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

func filterMap(m map[string]string, includeDotted bool) map[string]string {
	for k := range m {
		if strings.HasPrefix(k, tsuruLabelPrefix) {
			continue
		}
		if includeDotted != strings.Contains(k, ".") {
			delete(m, k)
		}
	}
	return m
}

func (n *kubernetesNodeWrapper) Metadata() map[string]string {
	return filterMap(n.allMetadata(), false)
}

func (n *kubernetesNodeWrapper) ExtraData() map[string]string {
	filteredMap := filterMap(n.allMetadata(), true)
	if n.cluster != nil {
		filteredMap[provision.LabelClusterMetadata] = n.cluster.Name
	}
	return filteredMap
}

func (n *kubernetesNodeWrapper) allMetadata() map[string]string {
	metadata := make(map[string]string, len(n.node.Labels)+len(n.node.Annotations))
	for k, v := range n.node.Annotations {
		metadata[k] = v
	}
	for k, v := range n.node.Labels {
		metadata[k] = v
	}
	return metadata
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
