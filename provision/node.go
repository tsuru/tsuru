// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"sort"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/net"
)

const (
	PoolMetadataName   = "pool"
	IaaSIDMetadataName = "iaas-id"
	IaaSMetadataName   = "iaas"
)

type MetaWithFrequency struct {
	Metadata map[string]string
	Nodes    []Node
}

type MetaWithFrequencyList []MetaWithFrequency

func (l MetaWithFrequencyList) Len() int           { return len(l) }
func (l MetaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l MetaWithFrequencyList) Less(i, j int) bool { return len(l[i].Nodes) < len(l[j].Nodes) }

type NodeList []Node

func FindNodeByAddrs(p NodeProvisioner, addrs []string) (Node, error) {
	nodeAddrMap := map[string]Node{}
	nodes, err := p.ListNodes(nil)
	if err != nil {
		return nil, err
	}
	for i, n := range nodes {
		nodeAddrMap[net.URLToHost(n.Address())] = nodes[i]
	}
	var node Node
	for _, addr := range addrs {
		n := nodeAddrMap[net.URLToHost(addr)]
		if n != nil {
			if node != nil {
				return nil, errors.Errorf("addrs match multiple nodes: %v", addrs)
			}
			node = n
		}
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}
	return node, nil
}

func FindNodeSkipProvisioner(address string, skipProv string) (Provisioner, Node, error) {
	provisioners, err := Registry()
	if err != nil {
		return nil, nil, err
	}
	provErrors := tsuruErrors.NewMultiError()
	for _, prov := range provisioners {
		if skipProv != "" && prov.GetName() == skipProv {
			continue
		}
		nodeProv, ok := prov.(NodeProvisioner)
		if !ok {
			continue
		}
		node, err := nodeProv.GetNode(address)
		if err == ErrNodeNotFound {
			continue
		}
		if err != nil {
			provErrors.Add(err)
			continue
		}
		return prov, node, nil
	}
	if provErrors.Len() > 0 {
		return nil, nil, provErrors
	}
	return nil, nil, ErrNodeNotFound
}

func FindNode(address string) (Provisioner, Node, error) {
	return FindNodeSkipProvisioner(address, "")
}

func metadataNoIaasID(n Node) map[string]string {
	// iaas-id is ignored because it wasn't created in previous tsuru versions
	// and having nodes with and without it would cause unbalanced metadata
	// errors.
	ignoredMetadata := []string{IaaSIDMetadataName}
	metadata := map[string]string{}
	for k, v := range n.MetadataNoPrefix() {
		metadata[k] = v
	}
	for _, val := range ignoredMetadata {
		delete(metadata, val)
	}
	return metadata
}

func (nodes NodeList) SplitMetadata() (MetaWithFrequencyList, map[string]string, error) {
	common := make(map[string]string)
	exclusive := make([]map[string]string, len(nodes))
	for i := range nodes {
		metadata := metadataNoIaasID(nodes[i])
		for k, v := range metadata {
			isExclusive := false
			for j := range nodes {
				if i == j {
					continue
				}
				otherMetadata := metadataNoIaasID(nodes[j])
				if v != otherMetadata[k] {
					isExclusive = true
					break
				}
			}
			if isExclusive {
				if exclusive[i] == nil {
					exclusive[i] = make(map[string]string)
				}
				exclusive[i][k] = v
			} else {
				common[k] = v
			}
		}
	}
	var group MetaWithFrequencyList
	sameMap := make(map[int]bool)
	for i := range exclusive {
		groupNodes := []Node{nodes[i]}
		for j := range exclusive {
			if i == j {
				continue
			}
			diffCount := 0
			for k, v := range exclusive[i] {
				if exclusive[j][k] != v {
					diffCount++
				}
			}
			if diffCount > 0 && (diffCount < len(exclusive[i]) || diffCount > len(exclusive[j])) {
				return nil, nil, errors.Errorf("unbalanced metadata for node group: %v vs %v", exclusive[i], exclusive[j])
			}
			if diffCount == 0 {
				sameMap[j] = true
				groupNodes = append(groupNodes, nodes[j])
			}
		}
		if !sameMap[i] && exclusive[i] != nil {
			group = append(group, MetaWithFrequency{Metadata: exclusive[i], Nodes: groupNodes})
		}
	}
	sort.Sort(group)
	return group, common, nil
}
