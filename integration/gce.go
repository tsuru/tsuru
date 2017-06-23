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

var zone = os.Getenv("GCE_ZONE")
var projectID = os.Getenv("GCE_PROJECT_ID")
var serviceAccount = os.Getenv("GCE_SERVICE_ACCOUNT")

// GceClusterManager represents a Google Compute Engine cluster (Container Engine)
type GceClusterManager struct {
	env         *Environment
	client      *gceClient
	clusterName string
	cluster     *container.Cluster
}

func newClusterName() string {
	return fmt.Sprintf("integration-test-%d", randInt())
}

func randInt() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Int()
}

func createTempFile(data []byte, prefix string) (string, error) {
	tmpfile, err := ioutil.TempFile("", prefix)
	if err != nil {
		return "", err
	}
	if _, err := tmpfile.Write(data); err != nil {
		return "", err
	}
	if err := tmpfile.Close(); err != nil {
		return "", err
	}
	return tmpfile.Name(), nil
}

func (g *GceClusterManager) Name() string {
	return "gce"
}

func (g *GceClusterManager) Provisioner() string {
	return "kubernetes"
}

func (g *GceClusterManager) IP() string {
	g.fetchClusterData()
	if g.cluster != nil {
		return g.cluster.Endpoint
	}
	return ""
}

func (g *GceClusterManager) Start() *Result {
	ctx := context.Background()
	serviceAccountFile, err := createTempFile([]byte(serviceAccount), "gce-sa-")
	if err != nil {
		return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] error creating service account file: %s", err)}
	}
	client, err := newClient(ctx, projectID, option.WithServiceAccountFile(serviceAccountFile))
	if err != nil {
		return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] error creating client: %s", err)}
	}
	g.client = client
	g.clusterName = g.env.Get("clustername")
	if g.clusterName == "" {
		g.clusterName = newClusterName()
		g.env.Set("clustername", g.clusterName)
		if g.env.VerboseLevel() > 0 {
			fmt.Fprintf(safeStdout, "[gce] starting cluster %s in zone %s\n", g.clusterName, zone)
		}
		g.client.createCluster(g.clusterName, zone, 1)
	} else {
		g.fetchClusterData()
		if g.cluster == nil || g.cluster.Status != gceClusterStatusRunning {
			return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] cluster %s is not running", g.clusterName)}
		}
	}
	return &Result{ExitCode: 0}
}

func (g *GceClusterManager) Delete() *Result {
	if g.clusterName == "" {
		return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] cluster name undefined")}
	}
	if g.env.VerboseLevel() > 0 {
		fmt.Fprintf(safeStdout, "[gce] deleting cluster %s in zone %s\n", g.clusterName, zone)
	}
	g.client.deleteCluster(g.cluster.Name, zone)
	return &Result{ExitCode: 0}
}

func (g *GceClusterManager) fetchClusterData() {
	if g.cluster != nil && g.cluster.Status == gceClusterStatusRunning {
		return
	}
	retries := 20
	sleepTime := 20 * time.Second
	for i := 0; i < retries; i++ {
		cluster, err := g.client.describeCluster(g.clusterName, zone)
		if err != nil {
			if g.env.VerboseLevel() > 0 {
				fmt.Fprintf(safeStdout, "[gce] error fetching cluster %s: %s\n", g.clusterName, err)
			}
			return
		}
		if cluster.Status == gceClusterStatusRunning {
			g.cluster = cluster
			if g.env.VerboseLevel() > 0 {
				fmt.Fprintf(safeStdout, "[gce] cluster %s is running. Endpoint: %s\n", g.clusterName, cluster.Endpoint)
			}
			return
		}
		if g.env.VerboseLevel() > 0 {
			fmt.Fprintf(safeStdout, "[gce] cluster %s status: %s\n", g.clusterName, cluster.Status)
		}
		time.Sleep(sleepTime)
	}
}

func (g *GceClusterManager) credentials() (map[string]string, error) {
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
	filename, err := createTempFile(contents, "gce-ca-")
	if err != nil {
		return credentials, err
	}
	credentials["certificateFilename"] = filename
	return credentials, nil
}

func (g *GceClusterManager) UpdateParams() []string {
	address := fmt.Sprintf("https://%s", g.IP())
	credentials, err := g.credentials()
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
