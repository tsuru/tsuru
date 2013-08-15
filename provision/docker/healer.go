// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/log"
)

type ContainerHealer struct{}

func (h *ContainerHealer) Heal() error {
	return nil
}

// collectContainers collect containers running in docker.
// It calls docker http api to accomplish the task.
func (h *ContainerHealer) collectContainers() ([]container, error) {
	opts := dockerClient.ListContainersOptions{All: true}
	apiContainers, err := dCluster.ListContainers(opts)
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
