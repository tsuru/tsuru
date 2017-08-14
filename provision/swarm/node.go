// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"

	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
)

type swarmNodeWrapper struct {
	*swarm.Node
	provisioner *swarmProvisioner
	client      *clusterClient
}

var (
	_ provision.Node = &swarmNodeWrapper{}
)

func (n *swarmNodeWrapper) Pool() string {
	l := provision.LabelSet{Labels: n.Node.Spec.Annotations.Labels, Prefix: tsuruLabelPrefix}
	return l.NodePool()
}

func (n *swarmNodeWrapper) Address() string {
	return nodeAddr(n.client, n.Node)
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
	labels := provision.LabelSet{Labels: n.Node.Spec.Annotations.Labels, Prefix: tsuruLabelPrefix}
	return labels.PublicNodeLabels()
}

func (n *swarmNodeWrapper) ExtraData() map[string]string {
	if n.client == nil {
		return nil
	}
	return map[string]string{
		provision.LabelClusterMetadata: n.client.Cluster.Name,
	}
}

func (n *swarmNodeWrapper) Units() ([]provision.Unit, error) {
	l := provision.LabelSet{Prefix: tsuruLabelPrefix}
	l.SetIsService()
	tasks, err := n.client.ListTasks(docker.ListTasksOptions{
		Filters: map[string][]string{
			"node":  {n.ID},
			"label": toLabelSelectors(l.ToIsServiceSelector()),
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tasksToUnits(n.client, tasks)
}

func (n *swarmNodeWrapper) Provisioner() provision.NodeProvisioner {
	return n.provisioner
}
