// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"io"
	"sort"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
)

type MetaWithFrequency struct {
	Metadata map[string]string
	Nodes    []provision.Node
}

type MetaWithFrequencyList []MetaWithFrequency

func (l MetaWithFrequencyList) Len() int           { return len(l) }
func (l MetaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l MetaWithFrequencyList) Less(i, j int) bool { return len(l[i].Nodes) < len(l[j].Nodes) }

type NodeList []provision.Node

func FindNodeByAddrs(p provision.NodeProvisioner, addrs []string) (provision.Node, error) {
	nodeAddrMap := map[string]provision.Node{}
	nodes, err := p.ListNodes(nil)
	if err != nil {
		return nil, err
	}
	for i, n := range nodes {
		nodeAddrMap[net.URLToHost(n.Address())] = nodes[i]
	}
	var node provision.Node
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
		return nil, provision.ErrNodeNotFound
	}
	return node, nil
}

func FindNodeSkipProvisioner(address string, skipProv string) (provision.Provisioner, provision.Node, error) {
	provisioners, err := provision.Registry()
	if err != nil {
		return nil, nil, err
	}
	provErrors := tsuruErrors.NewMultiError()
	for _, prov := range provisioners {
		if skipProv != "" && prov.GetName() == skipProv {
			continue
		}
		nodeProv, ok := prov.(provision.NodeProvisioner)
		if !ok {
			continue
		}
		node, err := nodeProv.GetNode(address)
		if err == provision.ErrNodeNotFound {
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
	return nil, nil, provision.ErrNodeNotFound
}

func FindNode(address string) (provision.Provisioner, provision.Node, error) {
	return FindNodeSkipProvisioner(address, "")
}

type RemoveNodeArgs struct {
	Node       provision.Node
	Prov       provision.NodeProvisioner
	Address    string
	Writer     io.Writer
	Rebalance  bool
	RemoveIaaS bool
}

func RemoveNode(args RemoveNodeArgs) error {
	var err error
	if args.Node == nil {
		if args.Prov == nil {
			return errors.New("arg Prov is required if Node is nil")
		}
		args.Node, err = args.Prov.GetNode(args.Address)
		if err != nil {
			return err
		}
	}
	return removeNodeWithNode(args.Node, provision.RemoveNodeOptions{
		Address:   args.Node.Address(),
		Rebalance: args.Rebalance,
		Writer:    args.Writer,
	}, args.RemoveIaaS)
}

func removeNodeWithNode(node provision.Node, opts provision.RemoveNodeOptions, removeIaaS bool) error {
	prov := node.Provisioner()
	err := prov.RemoveNode(opts)
	if err != nil {
		return err
	}
	multi := tsuruErrors.NewMultiError()
	err = healer.HealerInstance.RemoveNode(node)
	if err != nil {
		multi.Add(errors.Wrapf(err, "unable to remove healer data"))
	}
	if removeIaaS {
		var m iaas.Machine
		m, err = iaas.FindMachineByIdOrAddress(node.IaaSID(), net.URLToHost(opts.Address))
		if err == nil {
			err = m.Destroy()
		}
		if err != nil && err != iaas.ErrMachineNotFound {
			multi.Add(errors.Wrapf(err, "unable to destroy machine in iaas"))
		}
	}
	return multi.ToError()
}

func metadataNoIaasID(n provision.Node) map[string]string {
	// iaas-id is ignored because it wasn't created in previous tsuru versions
	// and having nodes with and without it would cause unbalanced metadata
	// errors.
	ignoredMetadata := []string{provision.IaaSIDMetadataName}
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
		groupNodes := []provision.Node{nodes[i]}
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

func HasAllMetadata(base, wanted map[string]string) bool {
	for key, value := range wanted {
		nodeVal := base[key]
		if nodeVal != value {
			return false
		}
	}
	return true
}

