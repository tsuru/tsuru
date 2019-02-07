// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"math/rand"
	"time"
)

type genericKubeCluster struct {
	name       string
	createData map[string]string
}

func newClusterName() string {
	name := fmt.Sprintf("kube-%d", randInt())
	if len(name) > 10 {
		name = name[:10]
	}
	return name
}

func randInt() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Int()
}

func (g *genericKubeCluster) Name() string {
	if g.name == "" {
		g.name = newClusterName()
	}
	return g.name
}

func (g *genericKubeCluster) Provisioner() string {
	return "kubernetes"
}

func (g *genericKubeCluster) Start() *Result {
	return &Result{}
}

func (g *genericKubeCluster) Delete() *Result {
	return &Result{}
}

func (g *genericKubeCluster) UpdateParams() ([]string, bool) {
	var clusterParts []string
	for k, v := range g.createData {
		clusterParts = append(clusterParts, "--create-data", fmt.Sprintf("%v=%v", k, v))
	}
	return clusterParts, false
}
