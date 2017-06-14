// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"

	"golang.org/x/net/context"
)

const gceClusterStatusRunning = "RUNNING"

var clusterName = fmt.Sprintf("integration-test-%d", randInt())
var zone = os.Getenv("GCE_ZONE")
var projectID = os.Getenv("GCE_PROJECT_ID")
var serviceAccountFile = os.Getenv("GCE_SERVICE_ACCOUNT_FILE")

type gceClusterManager struct {
	client  *gceClient
	cluster *container.Cluster
}

func randInt() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Int()
}

func (g *gceClusterManager) Name() string {
	return "gce"
}

func (g *gceClusterManager) Provisioner() string {
	return "kubernetes"
}

func (g *gceClusterManager) IP(env *Environment) string {
	g.fetchClusterData()
	if g.cluster != nil {
		return g.cluster.Endpoint
	}
	return ""
}

func (g *gceClusterManager) Start(env *Environment) *Result {
	ctx := context.Background()
	client, err := newClient(ctx, projectID, option.WithServiceAccountFile(serviceAccountFile))
	if err != nil {
		return nil
	}
	g.client = client
	g.client.createCluster(clusterName, zone, 1)
	return nil
}

func (g *gceClusterManager) Delete(env *Environment) *Result {
	g.client.deleteCluster(g.cluster.Name, zone)
	return nil
}

func (g *gceClusterManager) fetchClusterData() {
	if g.cluster != nil && g.cluster.Status == gceClusterStatusRunning {
		return
	}
	for i := 0; i < 10; i++ {
		cluster, err := g.client.describeCluster(clusterName, zone)
		if err == nil && cluster.Status == gceClusterStatusRunning {
			g.cluster = cluster
			return
		}
		time.Sleep(10 * time.Second)
	}
}

func (g *gceClusterManager) credentials(env *Environment) (map[string]string, error) {
	g.fetchClusterData()
	if g.cluster == nil {
		return nil, fmt.Errorf("cluster unavailable")
	}
	credentials := make(map[string]string)
	credentials["username"] = g.cluster.MasterAuth.Username
	credentials["password"] = g.cluster.MasterAuth.Password
	contents, err := base64.StdEncoding.DecodeString(g.cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return credentials, err
	}
	tmpfile, err := ioutil.TempFile("", "gce-ca")
	if err != nil {
		return credentials, err
	}
	if _, err := tmpfile.Write(contents); err != nil {
		return credentials, err
	}
	if err := tmpfile.Close(); err != nil {
		return credentials, err
	}
	credentials["certificateFilename"] = tmpfile.Name()
	return credentials, nil
}

func (g *gceClusterManager) UpdateParams(env *Environment) []string {
	address := fmt.Sprintf("https://%s", g.IP(env))
	credentials, err := g.credentials(env)
	if err != nil {
		return []string{}
	}
	return []string{
		"--addr", address,
		"--custom", "username=" + credentials["username"],
		"--custom", "password=" + credentials["password"],
		"--cacert", credentials["certificateFilename"],
	}
}
