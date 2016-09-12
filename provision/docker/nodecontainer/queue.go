// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"fmt"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
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
	node, err := t.provisioner.Cluster().GetNode(dockerEndpoint)
	if err != nil {
		job.Error(err)
		return
	}
	client, err := node.Client()
	if err != nil {
		job.Error(err)
		return
	}
	err = t.waitDocker(client)
	if err != nil {
		job.Error(err)
		return
	}
	node.CreationStatus = cluster.NodeCreationStatusCreated
	err = recreateContainers(t.provisioner, nil, node)
	if err != nil {
		t.provisioner.Cluster().UpdateNode(node)
		job.Error(err)
		return
	}
	node.Metadata["LastSuccess"] = time.Now().Format(time.RFC3339)
	_, err = t.provisioner.Cluster().UpdateNode(node)
	if err != nil {
		job.Error(err)
		return
	}
	job.Success(nil)
}

func (t *runBs) waitDocker(client *docker.Client) error {
	timeout, _ := config.GetInt("docker:api-timeout")
	if timeout == 0 {
		timeout = 600
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Second)
	pong := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		for {
			err := client.Ping()
			if err == nil {
				pong <- nil
				return
			}
			if e, ok := err.(*docker.Error); ok && e.Status > 499 {
				pong <- err
				return
			}
			select {
			case <-exit:
				return
			case <-time.After(time.Second):
			}
		}
	}()
	select {
	case err := <-pong:
		return err
	case <-timeoutChan:
		close(exit)
		return fmt.Errorf("Docker API at %q didn't respond after %d seconds", client.Endpoint(), timeout)
	}
}
