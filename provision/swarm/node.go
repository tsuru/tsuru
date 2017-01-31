// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
)

const (
	labelNodeInternalPrefix = "tsuru.internal."
)

var (
	labelNodeDockerAddr = tsuruLabel(labelNodeInternalPrefix + "docker-addr")
	labelNodePoolName   = tsuruLabel("pool")
)

type swarmNodeWrapper struct {
	*swarm.Node
	provisioner *swarmProvisioner
}

func (n *swarmNodeWrapper) Pool() string {
	return n.Node.Spec.Annotations.Labels[labelNodePoolName.String()]
}

func (n *swarmNodeWrapper) Address() string {
	return n.Node.Spec.Annotations.Labels[labelNodeDockerAddr.String()]
}

func (n *swarmNodeWrapper) Status() string {
	base := string(n.Node.Status.State)
	if n.Node.Status.Message != "" {
		base = fmt.Sprintf("%s (%s)", base, n.Node.Status.Message)
	}
	if n.Node.Spec.Availability != "" && n.Node.Spec.Availability != swarm.NodeAvailabilityActive {
		base = fmt.Sprintf("%s (%s)", base, n.Node.Spec.Availability)
	}
	return base
}

func (n *swarmNodeWrapper) Metadata() map[string]string {
	metadata := map[string]string{}
	for k, v := range n.Node.Spec.Annotations.Labels {
		if strings.HasPrefix(k, labelNodeInternalPrefix) {
			continue
		}
		metadata[k] = v
	}
	return metadata
}

func (n *swarmNodeWrapper) Units() ([]provision.Unit, error) {
	client, err := chooseDBSwarmNode()
	if err != nil {
		return nil, err
	}
	tasks, err := client.ListTasks(docker.ListTasksOptions{
		Filters: map[string][]string{
			"node":  {n.ID},
			"label": {fmt.Sprintf("%s=true", labelService)},
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tasksToUnits(client, tasks)
}

func (n *swarmNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.provisioner
}
