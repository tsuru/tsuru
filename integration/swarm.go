// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"strings"

	shellwords "github.com/mattn/go-shellwords"
	check "gopkg.in/check.v1"
)

type SwarmClusterManager struct {
	env *Environment
	c   *check.C
}

func (m *SwarmClusterManager) Name() string {
	return "swarm"
}

func (m *SwarmClusterManager) Provisioner() string {
	return swarmProvisioner
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

func (m *SwarmClusterManager) UpdateParams() ([]string, bool) {
	opts := nodeOrRegisterOpts(m.c, m.env)
	parts, _ := shellwords.Parse(opts)
	var clusterParts []string
	createNode := false
	for _, part := range parts {
		if part == "--register" {
			continue
		}
		metadata := strings.SplitN(part, "=", 2)
		if len(metadata) == 2 {
			if metadata[0] == "address" {
				clusterParts = append(clusterParts, "--addr", metadata[1])
			} else {
				createNode = true
				clusterParts = append(clusterParts, "--create-data", fmt.Sprintf("'%s'", part))
			}
		} else {
			clusterParts = append(clusterParts, part)
		}
	}
	return clusterParts, createNode
}
