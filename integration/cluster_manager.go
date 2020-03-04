// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import check "gopkg.in/check.v1"

const (
	kubernetesProvisioner = "kubernetes"
)

// ClusterManager is an abstraction to a Tsuru cluster
type ClusterManager interface {
	Name() string
	Provisioner() string
	Start() *Result
	Delete() *Result
	UpdateParams() (params []string, createNode bool)
}

func nodeOrRegisterOpts(c *check.C, env *Environment) string {
	opts := env.Get("nodeopts")
	if opts != "" {
		return opts
	}
	regOpts := env.All("noderegisteropts")
	c.Assert(regOpts, check.Not(check.HasLen), 0)
	env.Set("noderegisteropts", append(regOpts[1:], regOpts[0])...)
	return regOpts[0]
}
