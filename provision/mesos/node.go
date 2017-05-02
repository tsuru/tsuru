// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"fmt"

	"github.com/andygrunwald/megos"
	"github.com/tsuru/tsuru/provision"
)

type mesosNodeWrapper struct {
	slave   *megos.Slave
	cluster *clusterClient
	prov    *mesosProvisioner
}

func (n *mesosNodeWrapper) Pool() string {
	return ""
}

func (n *mesosNodeWrapper) Address() string {
	return n.slave.Hostname
}

func (n *mesosNodeWrapper) Status() string {
	return ""
}

func (n *mesosNodeWrapper) Metadata() map[string]string {
	metadata := map[string]string{}
	for k, v := range n.slave.Attributes {
		metadata[k] = fmt.Sprintf("%v", v)
	}
	return metadata
}

func (n *mesosNodeWrapper) Units() ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (n *mesosNodeWrapper) Provisioner() provision.NodeProvisioner {
	return nil
}
