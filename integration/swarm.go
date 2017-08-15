// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"strings"

	"github.com/mattn/go-shellwords"
)

type swarmClusterManager struct {
	env *Environment
}

func (m *swarmClusterManager) Name() string {
	return "swarm"
}

func (m *swarmClusterManager) Provisioner() string {
	return "swarm"
}

func (m *swarmClusterManager) Start() *Result {
	return &Result{}
}

func (m *swarmClusterManager) Delete() *Result {
	return &Result{}
}

func (m *swarmClusterManager) RequiredNodes() int {
	return 1
}

func (m *swarmClusterManager) UpdateParams() []string {
	nodeopts := m.env.All("nodeopts")
	m.env.Set("nodeopts", append(nodeopts[1:], nodeopts[0])...)
	parts, _ := shellwords.Parse(nodeopts[0])
	var clusterParts []string
	for _, part := range parts {
		if part == "--register" {
			continue
		}
		metadata := strings.SplitN(part, "=", 2)
		if len(metadata) == 2 {
			if metadata[0] == "address" {
				clusterParts = append(clusterParts, "--addr", metadata[1])
			} else {
				clusterParts = append(clusterParts, "--create-data", part)
			}
		} else {
			clusterParts = append(clusterParts, part)
		}
	}
	return clusterParts
}
