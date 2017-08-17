// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"strings"

	"github.com/mattn/go-shellwords"
)

type SwarmClusterManager struct {
	env *Environment
}

func (m *SwarmClusterManager) Name() string {
	return "swarm"
}

func (m *SwarmClusterManager) Provisioner() string {
	return "swarm"
}

func (m *SwarmClusterManager) Start() *Result {
	return &Result{}
}

func (m *SwarmClusterManager) Delete() *Result {
	return &Result{}
}

func (m *SwarmClusterManager) RequiredNodes() int {
	return 1
}

func (m *SwarmClusterManager) UpdateParams() []string {
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
				clusterParts = append(clusterParts, "--create-data", fmt.Sprintf("'%s'", part))
			}
		} else {
			clusterParts = append(clusterParts, part)
		}
	}
	return clusterParts
}
