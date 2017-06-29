// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import "strings"

func (s *S) getPlatforms() []string {
	availablePlatforms := []string{
		"python",
		"go",
		"buildpack",
		"cordova",
		"elixir",
		"java",
		"nodejs",
		"php",
		"play",
		"pypy",
		"python3",
		"ruby",
		"static",
	}
	platforms := s.env.All("platforms")
	selectedPlatforms := make([]string, 0, len(availablePlatforms))
	for _, platform := range platforms {
		platform = strings.Trim(platform, " ")
		for i, item := range availablePlatforms {
			if item == platform {
				selectedPlatforms = append(selectedPlatforms, "tsuru/"+platform)
				availablePlatforms = append(availablePlatforms[:i], availablePlatforms[i+1:]...)
				break
			}
		}
	}
	if len(selectedPlatforms) == 0 {
		return availablePlatforms
	}
	return selectedPlatforms
}

func (s *S) getProvisioners() []string {
	availableProvisioners := []string{"docker", "swarm"}
	provisioners := s.env.All("provisioners")
	selectedProvisioners := make([]string, 0, len(availableProvisioners))
	for _, provisioner := range provisioners {
		provisioner = strings.Trim(provisioner, " ")
		for i, item := range availableProvisioners {
			if item == provisioner {
				selectedProvisioners = append(selectedProvisioners, provisioner)
				availableProvisioners = append(availableProvisioners[:i], availableProvisioners[i+1:]...)
				break
			}
		}
	}
	if len(selectedProvisioners) == 0 {
		return availableProvisioners
	}
	return selectedProvisioners
}

func (s *S) getClusterManagers() []ClusterManager {
	availableClusterManagers := map[string]ClusterManager{
		"gce":      &GceClusterManager{env: s.env},
		"minikube": &MinikubeClusterManager{env: s.env},
	}
	managers := make([]ClusterManager, 0, len(availableClusterManagers))
	clusters := s.env.All("clusters")
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

func installerName(env *Environment) string {
	name := env.Get("installername")
	if name == "" {
		name = "tsuru"
	}
	return name
}

func (s *S) config() {
	env := NewEnvironment()
	if !env.Has("enabled") {
		return
	}
	s.env = env
	allPlatforms = s.getPlatforms()
	allProvisioners = s.getProvisioners()
	clusterManagers = s.getClusterManagers()
}
