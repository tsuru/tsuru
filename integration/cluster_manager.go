// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

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
