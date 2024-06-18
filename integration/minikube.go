// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"regexp"
)

// MinikubeClusterManager represents a minikube local instance
type MinikubeClusterManager struct {
	env       *Environment
	ipAddress string
}

func (m *MinikubeClusterManager) Name() string {
	return "minikube"
}

func (m *MinikubeClusterManager) Provisioner() string {
	return kubernetesProvisioner
}

func (m *MinikubeClusterManager) IP() string {
	if len(m.ipAddress) == 0 {
		minikube := NewCommand("minikube").WithArgs
		res := minikube("ip").Run(m.env)
		if res.Error != nil || res.ExitCode != 0 {
			return ""
		}
		regex := regexp.MustCompile(`([\d.]+)`)
		parts := regex.FindStringSubmatch(res.Stdout.String())
		if len(parts) != 2 {
			return ""
		}
		m.ipAddress = parts[1]
	}
	return m.ipAddress
}

func (m *MinikubeClusterManager) Start() *Result {
	return &Result{}
}

func (m *MinikubeClusterManager) Delete() *Result {
	return &Result{}
}

func (m *MinikubeClusterManager) UpdateParams() ([]string, bool) {
	return nil, false
}
