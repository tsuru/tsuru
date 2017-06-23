// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

type clusterManager interface {
	Name() string
	Provisioner() string
	IP(env *Environment) string
	Start(env *Environment) *Result
	Delete(env *Environment) *Result
	UpdateParams(env *Environment) []string
}
