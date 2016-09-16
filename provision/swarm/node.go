// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"

	"github.com/docker/engine-api/types/swarm"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
)

type swarmNodeWrapper struct {
	*swarm.Node
}

func (n *swarmNodeWrapper) Pool() string {
	return n.Node.Spec.Annotations.Labels["pool"]
}

func (n *swarmNodeWrapper) Address() string {
	host := tsuruNet.URLToHost(n.Node.ManagerStatus.Addr)
	return fmt.Sprintf("%s:%d", host, swarmConfig.dockerPort)
}

func (n *swarmNodeWrapper) Status() string {
	base := string(n.Node.Status.State)
	if n.Node.Status.Message != "" {
		return fmt.Sprintf("%s (%s)", base, n.Node.Status.Message)
	}
	return base
}

func (n *swarmNodeWrapper) Metadata() map[string]string {
	return n.Node.Spec.Annotations.Labels
}

func (n *swarmNodeWrapper) Units() ([]provision.Unit, error) {
	return nil, nil
}
