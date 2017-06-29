// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"os"
	"strings"
)

func (s *S) getInstallerConfig() string {
	// if Docker provisioner is not set, add an extra host, so Tsuru can build platforms
	hosts := len(allProvisioners) + 1
	for _, provisioner := range allProvisioners {
		if provisioner == "docker" {
			hosts--
			break
		}
	}
	return fmt.Sprintf(`driver:
  name: virtualbox
  options:
    virtualbox-cpu-count: 2
    virtualbox-memory: 2048
docker-flags:
  - experimental
hosts:
  apps:
    size: %d
components:
  install-dashboard: false
`, hosts)
}

func (s *S) getPlatforms() []string {
	availablePlatforms := []string{
		"tsuru/python",
		"tsuru/go",
		"tsuru/buildpack",
		"tsuru/cordova",
		"tsuru/elixir",
		"tsuru/java",
		"tsuru/nodejs",
		"tsuru/php",
		"tsuru/play",
		"tsuru/pypy",
		"tsuru/python3",
		"tsuru/ruby",
		"tsuru/static",
	}
	if _, ok := os.LookupEnv(integrationEnvID + "platforms"); !ok {
		return availablePlatforms
	}
	platforms := s.env.All("platforms")
	selectedPlatforms := make([]string, 0, len(availablePlatforms))
	for _, name := range platforms {
		name = strings.Trim(name, " ")
		for i, platform := range availablePlatforms {
			if name == platform || "tsuru/"+name == platform {
				selectedPlatforms = append(selectedPlatforms, platform)
				availablePlatforms = append(availablePlatforms[:i], availablePlatforms[i+1:]...)
				break
			}
		}
	}
	return selectedPlatforms
}

func (s *S) getProvisioners() []string {
	availableProvisioners := []string{"docker", "swarm"}
	if _, ok := os.LookupEnv(integrationEnvID + "provisioners"); !ok {
		return availableProvisioners
	}
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
	installerConfig = s.getInstallerConfig()
}
