// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import "github.com/tsuru/tsuru/provision"

type kubernetesNodeWrapper struct {
	Addresses []string
}

func (n *kubernetesNodeWrapper) Pool() string {
	return ""
}

func (n *kubernetesNodeWrapper) Address() string {
	return n.Addresses[0]
}

func (n *kubernetesNodeWrapper) Status() string {
	return ""
}

func (n *kubernetesNodeWrapper) Metadata() map[string]string {
	return nil
}

func (n *kubernetesNodeWrapper) Units() ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (n *kubernetesNodeWrapper) Provisioner() provision.NodeProvisioner {
	return nil
}
