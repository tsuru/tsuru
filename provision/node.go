// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/net"
)

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
