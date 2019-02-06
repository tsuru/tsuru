// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"math/rand"
	"os"
	"time"
)

type GKEClusterManager struct {
	name string
}

func newClusterName() string {
	name := fmt.Sprintf("igke-%d", randInt())
	if len(name) > 10 {
		name = name[:10]
	}
	return name
}

func randInt() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Int()
}

func (g *GKEClusterManager) Name() string {
	if g.name == "" {
		g.name = newClusterName()
	}
	return g.name
}

func (g *GKEClusterManager) Provisioner() string {
	return "kubernetes"
}

func (g *GKEClusterManager) Start() *Result {
	return &Result{}
}

func (g *GKEClusterManager) Delete() *Result {
	return &Result{}
}

func (g *GKEClusterManager) UpdateParams() ([]string, bool) {
	clusterParts := []string{
		"--create-data", "driver=googlekubernetesengine",
		"--create-data", "node-count=1",
		"--create-data", "zone=" + os.Getenv("GCE_ZONE"),
		"--create-data", "project-id=" + os.Getenv("GCE_PROJECT_ID"),
		"--create-data", "machine-type=" + os.Getenv("GCE_MACHINE_TYPE"),
	}
	return clusterParts, false
}
