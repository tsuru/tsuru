// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import "strings"

type clusterManager interface {
	Name() string
	Provisioner() string
	IP(env *Environment) string
	Start(env *Environment) *Result
	Delete(env *Environment) *Result
	UpdateParams(env *Environment) []string
}

func getClusterManagers(env *Environment) []clusterManager {
	availableClusterManagers := map[string]clusterManager{
		"gce":      &gceClusterManager{},
		"minikube": &minikubeClusterManager{},
	}
	managers := make([]clusterManager, 0, len(availableClusterManagers))
	clusters := strings.Split(env.Get("clusters"), ",")
	selectedClusters := make([]string, 0, len(availableClusterManagers))
	for _, cluster := range clusters {
		cluster = strings.Trim(cluster, " ")
		manager := availableClusterManagers[cluster]
		if manager != nil {
			available := true
			for _, selected := range selectedClusters {
				if cluster == selected {
					available = false
					break
				}
			}
			if available {
				managers = append(managers, manager)
				selectedClusters = append(selectedClusters, cluster)
			}
		}
	}
	return managers
}
