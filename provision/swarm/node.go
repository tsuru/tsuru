// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/tsuru/tsuru/provision"
)

type swarmNodeWrapper struct {
	*swarm.Node
}

func (n *swarmNodeWrapper) Pool() string {
	return n.Node.Spec.Annotations.Labels["pool"]
}

func (n *swarmNodeWrapper) Address() string {
	return n.Node.Spec.Annotations.Labels[labelDockerAddr]
}

func (n *swarmNodeWrapper) Status() string {
	base := string(n.Node.Status.State)
	if n.Node.Status.Message != "" {
		return fmt.Sprintf("%s (%s)", base, n.Node.Status.Message)
	}
	return base
}

func (n *swarmNodeWrapper) Metadata() map[string]string {
	metadata := map[string]string{}
	for k, v := range n.Node.Spec.Annotations.Labels {
		if strings.HasPrefix(k, labelInternalPrefix) {
			continue
		}
		metadata[k] = v
	}
	return metadata
}

func (n *swarmNodeWrapper) Units() ([]provision.Unit, error) {
	return nil, errNotImplemented
}
