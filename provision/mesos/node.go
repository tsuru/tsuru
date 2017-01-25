// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import "github.com/tsuru/tsuru/provision"

type mesosNodeWrapper struct {
	Addresses []string
}

func (n *mesosNodeWrapper) Pool() string {
	return ""
}

func (n *mesosNodeWrapper) Address() string {
	return n.Addresses[0]
}

func (n *mesosNodeWrapper) Status() string {
	return ""
}

func (n *mesosNodeWrapper) Metadata() map[string]string {
	return nil
}

func (n *mesosNodeWrapper) Units() ([]provision.Unit, error) {
	return nil, errNotImplemented
}

func (n *mesosNodeWrapper) Provisioner() provision.NodeProvisioner {
	return nil
}
