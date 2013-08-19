// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/heal"
	"github.com/globocom/tsuru/log"
	"strings"
)

func init() {
	heal.Register("docker", "container", ContainerHealer{})
}

type ContainerHealer struct{}

func (h ContainerHealer) Heal() error {
	containers, err := h.collectContainers()
	if err != nil {
		return err
	}
	unhealthy := h.unhealthyRunningContainers(containers)
	for _, c := range unhealthy {
		log.Printf("Attempting to heal container %s", c.ID)
		if err := dockerCluster().KillContainer(c.ID); err != nil {
			log.Printf("Caught error while killing container %s for healing: %s", c.ID, err.Error())
			continue
		}
		if err := dockerCluster().StartContainer(c.ID); err != nil {
			log.Printf("Caught error while starting container %s for healing: %s", c.ID, err.Error())
		}
	}
	return nil
}

// collectContainers collect and returns all containers running in docker.
// It calls docker http api to accomplish the task.
func (h ContainerHealer) collectContainers() ([]container, error) {
	opts := dockerClient.ListContainersOptions{All: true}
	apiContainers, err := dockerCluster().ListContainers(opts)
	if err != nil {
		log.Printf("Caught error while listing containers: %s", err.Error())
		return nil, err
	}
	containers := make([]container, len(apiContainers))
	for i, apiContainer := range apiContainers {
		c := container{
			ID:     apiContainer.ID,
			Image:  apiContainer.Image,
			Status: apiContainer.Status,
		}
		containers[i] = c
	}
	return containers, nil
}

// isHealthy analyses the health of a given container.
// It considers the container.Status field, if it is not up
// it will return false, otherwise it returns true.
func (h ContainerHealer) isHealthy(c *container) bool {
	return strings.Contains(c.Status, "Up") || c.Status == "Exit 0"
}

// isRunning checks whether a container is up or not and returns
// a boolean indicating the result.
// It analyses the container.Status field, returning false when it is exited
// and true otherwise.
func (h ContainerHealer) isRunning(c *container) bool {
	return !strings.Contains(c.Status, "Exit")
}

// unhealthyRunningContainers returns a list of unhealthy containers.
// It uses ContainerHealer.isHealthy method to filter containers.
func (h ContainerHealer) unhealthyRunningContainers(containers []container) []container {
	unhealthy := []container{}
	for _, c := range containers {
		if !h.isHealthy(&c) && h.isRunning(&c) {
			unhealthy = append(unhealthy, c)
		}
	}
	return unhealthy
}
