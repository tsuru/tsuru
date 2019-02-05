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

type GceClusterManager struct{}

func newClusterName() string {
	name := fmt.Sprintf("integration-test-gce-%d", randInt())
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func randInt() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Int()
}

func (g *GceClusterManager) Name() string {
	return newClusterName()
}

func (g *GceClusterManager) Provisioner() string {
	return "kubernetes"
}

func (g *GceClusterManager) Start() *Result {
	return &Result{}
}

func (g *GceClusterManager) Delete() *Result {
	return &Result{}
}

func (g *GceClusterManager) UpdateParams() ([]string, bool) {
	clusterParts := []string{
		"--create-data", "driver=googlekubernetesengine",
		"--create-data", "node-count=1",
		"--create-data", "zone=" + os.Getenv("GCE_ZONE"),
		"--create-data", "project-id=" + os.Getenv("GCE_PROJECT_ID"),
		"--create-data", "credential=" + os.Getenv("GCE_SERVICE_ACCOUNT"),
		"--create-data", "machine-type=" + os.Getenv("GCE_MACHINE_TYPE"),
	}
	return clusterParts, false
}
