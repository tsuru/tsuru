// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/provision/dockercommon"
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
	err = dockercommon.WaitDocker(client)
	if err != nil {
		job.Error(err)
		return
	}
	var recreateErr error
	node, err = t.provisioner.Cluster().AtomicUpdateNode(dockerEndpoint, func(node cluster.Node) (cluster.Node, error) {
		if node.CreationStatus != cluster.NodeCreationStatusPending {
			return cluster.Node{}, errors.Errorf("invalid node creation status: %q", node.CreationStatus)
		}
		node.CreationStatus = cluster.NodeCreationStatusCreated
		recreateErr = recreateContainers(t.provisioner, nil, node)
		if recreateErr == nil {
			node.Metadata["LastSuccess"] = time.Now().Format(time.RFC3339)
		}
		return node, nil
	})
	if err == nil {
		err = recreateErr
	}
	if err != nil {
		job.Error(err)
		return
	}
	job.Success(nil)
}
