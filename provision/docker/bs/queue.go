// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"fmt"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

const QueueTaskName = "run-bs"

// RegisterQueueTask registers the internal bs queue task for later execution.
func RegisterQueueTask(p DockerProvisioner) error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	return q.RegisterTask(&runBs{provisioner: p})
}

type runBs struct {
	provisioner DockerProvisioner
}

func (t *runBs) Name() string {
	return QueueTaskName
}

func (t *runBs) Run(job monsterqueue.Job) {
	params := job.Parameters()
	dockerEndpoint := params["endpoint"].(string)
	machineID := params["machine"].(string)
	node := cluster.Node{Address: dockerEndpoint}
	err := t.waitDocker(dockerEndpoint)
	if err != nil {
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	rawMetadata := params["metadata"].(monsterqueue.JobParams)
	metadata := make(map[string]string, len(rawMetadata))
	for key, value := range rawMetadata {
		metadata[key] = value.(string)
	}
	err = createContainer(dockerEndpoint, metadata["pool"], t.provisioner, true)
	if err != nil {
		node.CreationStatus = cluster.NodeCreationStatusError
		node.Metadata = map[string]string{"creationError": err.Error()}
		t.provisioner.Cluster().UpdateNode(node)
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	node.CreationStatus = cluster.NodeCreationStatusCreated
	_, err = t.provisioner.Cluster().UpdateNode(node)
	if err != nil {
		job.Error(err)
		t.destroyMachine(machineID)
		return
	}
	job.Success(nil)
}

func (t *runBs) waitDocker(endpoint string) error {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return err
	}
	timeout, _ := config.GetInt("docker:api-timeout")
	if timeout == 0 {
		timeout = 600
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Second)
	pong := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		for {
			select {
			case <-exit:
				return
			default:
				err := client.Ping()
				if err == nil {
					pong <- nil
					return
				}
				if e, ok := err.(*docker.Error); ok && e.Status > 499 {
					pong <- err
					return
				}
			}
		}
	}()
	select {
	case err := <-pong:
		return err
	case <-timeoutChan:
		close(exit)
		return fmt.Errorf("Docker API at %q didn't respond after %d seconds", endpoint, timeout)
	}
}

func (t *runBs) destroyMachine(id string) {
	if id != "" {
		machine, err := iaas.FindMachineById(id)
		if err != nil {
			log.Errorf("failed to remove machine %q: %s", id, err)
			return
		}
		err = machine.Destroy()
		if err != nil {
			log.Errorf("failed to remove machine %q: %s", id, err)
			return
		}
	}
}
