// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"os"
	"strings"
)

type clusterManager interface {
	Name() string
	Provisioner() string
	IP(env *Environment) string
	Start(env *Environment) *Result
	Delete(env *Environment) *Result
	UpdateParams(env *Environment) []string
}

var availableClusterManagers = map[string]clusterManager{
	"gce":      &gceClusterManager{},
	"minikube": &minikubeClusterManager{},
}

func getClusterManagers() []clusterManager {
	managers := make([]clusterManager, 0, len(availableClusterManagers))
	env := os.Getenv("TSURU_INTEGRATION_CLUSTERS")
	clusters := strings.Split(env, ",")
	for _, cluster := range clusters {
		cluster = strings.Trim(cluster, " ")
		manager := availableClusterManagers[cluster]
		if manager != nil {
			managers = append(managers, manager)
		}
	}
	return managers
}
