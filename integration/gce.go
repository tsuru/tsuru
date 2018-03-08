// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

const gceClusterStatusRunning = "RUNNING"

// GceClusterManager represents a Google Compute Engine cluster (Container Engine)
type GceClusterManager struct {
	env         *Environment
	client      *gceClient
	clusterName string
	cluster     *container.Cluster
	zone        string
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
	g.zone = os.Getenv("GCE_ZONE")
	projectID := os.Getenv("GCE_PROJECT_ID")
	serviceAccount := os.Getenv("GCE_SERVICE_ACCOUNT")
	machineType := os.Getenv("GCE_MACHINE_TYPE")
	if machineType == "" {
		machineType = "n1-standard-4"
	}
	serviceAccountFile, err := createTempFile([]byte(serviceAccount), "gce-sa-")
	if err != nil {
		return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] error creating service account file: %s", err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	client, err := newClient(ctx, projectID, option.WithServiceAccountFile(serviceAccountFile))
	cancel()
	if err != nil {
		return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] error creating client: %s", err)}
	}
	g.client = client
	g.clusterName = g.env.Get("clustername")
	if g.clusterName == "" {
		g.clusterName = newClusterName()
		if g.env.VerboseLevel() > 0 {
			fmt.Fprintf(safeStdout, "[gce] starting cluster %s in zone %s\n", g.clusterName, g.zone)
		}
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute*15)
		g.client.createCluster(ctx, g.clusterName, g.zone, machineType, 1)
		cancel()
	} else {
		g.fetchClusterData()
		if g.cluster == nil || g.cluster.Status != gceClusterStatusRunning {
			return &Result{ExitCode: 1, Error: fmt.Errorf("[gce] cluster %s is not running", g.clusterName)}
		}
	}
	return &Result{ExitCode: 0}
}

func (g *GceClusterManager) Delete() *Result {
	if g.env.Get("clustername") != "" {
		return &Result{ExitCode: 0}
	}
	if g.client == nil || g.cluster == nil {
		return &Result{ExitCode: 0}
	}
	if g.env.VerboseLevel() > 0 {
		fmt.Fprintf(safeStdout, "[gce] deleting cluster %s in zone %s\n", g.clusterName, g.zone)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	g.client.deleteCluster(ctx, g.cluster.Name, g.zone)
	cancel()
	return &Result{ExitCode: 0}
}

func (g *GceClusterManager) fetchClusterData() {
	if g.cluster != nil && g.cluster.Status == gceClusterStatusRunning {
		return
	}
	retries := 20
	sleepTime := 20 * time.Second
	for i := 0; i < retries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		cluster, err := g.client.describeCluster(ctx, g.clusterName, g.zone)
		cancel()
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

func (g *GceClusterManager) UpdateParams() ([]string, bool) {
	address := fmt.Sprintf("https://%s", g.IP())
	credentials, err := g.credentials()
	if err != nil {
		return []string{}, false
	}
	return []string{
		"--addr", address,
		"--custom", "username=" + credentials["username"],
		"--custom", "password=" + credentials["password"],
		"--cacert", credentials["certificateFilename"],
	}, false
}
