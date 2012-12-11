// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/provision"
	"io"
)

func init() {
	provision.Register("juju", &JujuProvisioner{})
}

// JujuProvisioner is an implementation for Provisioner interface. For more
// details on how a provisioner work, check the documentation of the provision
// package.
type JujuProvisioner struct{}

func (p *JujuProvisioner) Provision(app provision.App) error {
	return nil
}

func (p *JujuProvisioner) Destroy(app provision.App) error {
	return nil
}

func (p *JujuProvisioner) ExecuteCommand(w io.Writer, app provision.App, cmd string, args ...string) error {
	return nil
}

func (p *JujuProvisioner) CollectStatus() []provision.Unit {
	return nil
}
